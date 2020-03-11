/*****************************************************************************
 * RRDtool 1.4.9  Copyright by Tobi Oetiker, 1997-2014
 *****************************************************************************
 * rrd_create.c  creates new rrds
 *****************************************************************************/

#include <stdlib.h>
#include <time.h>
#include <locale.h>

#include "rrd_error.h"
#include "rrd_tool.h"
#include "rrd_rpncalc.h"
#include "rrd_hw.h"
#ifndef RRD_LITE
#include "rrd_client.h"
#endif
#include "rrd_config.h"

#include "rrd_is_thread_safe.h"
static int opt_no_overwrite = 0;

#ifdef WIN32
# include <process.h>
#endif

unsigned long FnvHash( const char *str);
int create_hw_contingent_rras( rrd_t *rrd, 
	unsigned short period, unsigned long hashed_name);
int parseGENERIC_DS( const char *def, rrd_t *rrd, int ds_idx);
static void rrd_free2( rrd_t *rrd);        /* our onwn copy, immmune to mmap */

/* #define DEBUG */
int rrd_create_r( const char *filename, unsigned long pdp_step,
		time_t last_up, int argc, const char **argv) {
	rrd_t     rrd;
	long      i;
	int       offset;
	char     *token;
	char      dummychar1[2], dummychar2[2];
	unsigned short token_idx, error_flag, period = 0;
	unsigned long hashed_name;
	int       ret = 0;

	/* init rrd clean */
	rrd_init(&rrd);
	/* static header */
	if ((rrd.stat_head = (stat_head_t*)calloc(1, sizeof(stat_head_t))) == NULL) {
		rrd_free2(&rrd);
		return -RRD_ERR_ALLOC;
	}

	/* live header */
	if ((rrd.live_head = (live_head_t*)calloc(1, sizeof(live_head_t))) == NULL) {
		rrd_free2(&rrd);
		return -RRD_ERR_ALLOC;
	}

	/* set some defaults */
	strcpy(rrd.stat_head->cookie, RRD_COOKIE);
	strcpy(rrd.stat_head->version, RRD_VERSION3);   /* by default we are still version 3 */
	rrd.stat_head->float_cookie = FLOAT_COOKIE;
	rrd.stat_head->ds_cnt = 0;  /* this will be adjusted later */
	rrd.stat_head->rra_cnt = 0; /* ditto */
	rrd.stat_head->pdp_step = pdp_step; /* 5 minute default */

	/* a default value */
	rrd.ds_def = NULL;
	rrd.rra_def = NULL;

	rrd.live_head->last_up = last_up;

	/* optind points to the first non-option command line arg,
	 * in this case, the file name. */
	/* Compute the FNV hash value (used by SEASONAL and DEVSEASONAL
	 * arrays. */
	hashed_name = FnvHash(filename);
	for (i = 0; i < argc; i++) {
		unsigned int ii;

		if (strncmp(argv[i], "DS:", 3) == 0) {
			size_t    old_size = sizeof(ds_def_t) * (rrd.stat_head->ds_cnt);

			if ((rrd.ds_def = (ds_def_t*)rrd_realloc(rrd.ds_def,
							old_size + sizeof(ds_def_t))) ==
					NULL) {
				rrd_free2(&rrd);
				return -RRD_ERR_ALLOC;
			}
			memset(&rrd.ds_def[rrd.stat_head->ds_cnt], 0, sizeof(ds_def_t));
			/* extract the name and type */
			switch (sscanf(&argv[i][3],
						DS_NAM_FMT "%1[:]" DST_FMT "%1[:]%n",
						rrd.ds_def[rrd.stat_head->ds_cnt].ds_nam,
						dummychar1,
						rrd.ds_def[rrd.stat_head->ds_cnt].dst,
						dummychar2, &offset)) {
				case 0:
				case 1:
					ret = -RRD_ERR_INVALID_DS_NAME;
					break;
				case 2:
				case 3:
					ret = -RRD_ERR_INVALID_DS_TYPE;
					break;
				case 4:    /* (%n may or may not be counted) */
				case 5:    /* check for duplicate datasource names */
					for (ii = 0; ii < rrd.stat_head->ds_cnt; ii++)
						if (strcmp(rrd.ds_def[rrd.stat_head->ds_cnt].ds_nam,
									rrd.ds_def[ii].ds_nam) == 0)
							ret = -RRD_ERR_DUPLICATE_DS_NAME;
					/* DS_type may be valid or not. Checked later */
					break;
				default:
					ret = -RRD_ERR_INVALID_DS_FORMAT;
			}
			if (ret) {
				rrd_free2(&rrd);
				return ret;
			}

			/* parse the remainder of the arguments */
			switch (dst_conv(rrd.ds_def[rrd.stat_head->ds_cnt].dst)) {
				case DST_COUNTER:
				case DST_ABSOLUTE:
				case DST_GAUGE:
				case DST_DERIVE:
					ret = parseGENERIC_DS(&argv[i][offset + 3], &rrd,
							rrd.stat_head->ds_cnt);
					break;
				case DST_CDEF:
					ret = parseCDEF_DS(&argv[i][offset + 3], &rrd,
							rrd.stat_head->ds_cnt);
					break;
				default:
					ret = -RRD_ERR_INVALID_DS_TYPE_SPEC;
					break;
			}

			if (ret) {
				rrd_free2(&rrd);
				return ret;
			}
			rrd.stat_head->ds_cnt++;
		} else if (strncmp(argv[i], "RRA:", 4) == 0) {
			char     *argvcopy;
			char     *tokptr = "";
			int       cf_id = -1;
			size_t    old_size = sizeof(rra_def_t) * (rrd.stat_head->rra_cnt);
			int       row_cnt;
			int       token_min = 4;
			if ((rrd.rra_def = (rra_def_t*)rrd_realloc(rrd.rra_def,
							old_size + sizeof(rra_def_t))) ==
					NULL) {
				rrd_free2(&rrd);
				return -RRD_ERR_ALLOC;
			}
			memset(&rrd.rra_def[rrd.stat_head->rra_cnt], 0,
					sizeof(rra_def_t));

			argvcopy = strdup(argv[i]);
			token = strtok_r(&argvcopy[4], ":", &tokptr);
			token_idx = error_flag = 0;

			while (token != NULL) {
				switch (token_idx) {
					case 0:
						if (sscanf(token, CF_NAM_FMT,
									rrd.rra_def[rrd.stat_head->rra_cnt].cf_nam) !=
								1)
							ret = -RRD_ERR_FAILED_PARSE_CF_NAME;
						cf_id = cf_conv(rrd.rra_def[rrd.stat_head->rra_cnt].cf_nam);
						switch (cf_id) {
							case CF_MHWPREDICT:
								strcpy(rrd.stat_head->version, RRD_VERSION);    /* MHWPREDICT causes Version 4 */
							case CF_HWPREDICT:
								token_min = 5;
								/* initialize some parameters */
								rrd.rra_def[rrd.stat_head->rra_cnt].par[RRA_hw_alpha].
									u_val = 0.1;
								rrd.rra_def[rrd.stat_head->rra_cnt].par[RRA_hw_beta].
									u_val = 1.0 / 288;
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_dependent_rra_idx].u_cnt =
									rrd.stat_head->rra_cnt;
								break;
							case CF_DEVSEASONAL:
								token_min = 3;
							case CF_SEASONAL:
								if (cf_id == CF_SEASONAL){
									token_min = 4;
								}
								/* initialize some parameters */
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_seasonal_gamma].u_val = 0.1;
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_seasonal_smoothing_window].u_val = 0.05;
								/* fall through */
							case CF_DEVPREDICT:
								if (cf_id == CF_DEVPREDICT){
									token_min = 3;
								}
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_dependent_rra_idx].u_cnt = -1;
								break;
							case CF_FAILURES:
								token_min = 5;
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_delta_pos].u_val = 2.0;
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_delta_neg].u_val = 2.0;
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_window_len].u_cnt = 3;
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_failure_threshold].u_cnt = 2;
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_dependent_rra_idx].u_cnt = -1;
								break;
								/* invalid consolidation function */
							case -1:
								ret = -RRD_ERR_UNREC_CONSOLIDATION_FUNC;
							default:
								break;
						}
						/* default: 1 pdp per cdp */
						rrd.rra_def[rrd.stat_head->rra_cnt].pdp_cnt = 1;
						break;
					case 1:
						switch (cf_conv(rrd.rra_def[rrd.stat_head->rra_cnt].cf_nam)) {
							case CF_HWPREDICT:
							case CF_MHWPREDICT:
							case CF_DEVSEASONAL:
							case CF_SEASONAL:
							case CF_DEVPREDICT:
							case CF_FAILURES:
								row_cnt = atoi(token);
								if (row_cnt <= 0)
									ret = -RRD_ERR_INVALID_ROW_COUNT;
								rrd.rra_def[rrd.stat_head->rra_cnt].row_cnt = row_cnt;
								break;
							default:
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_cdp_xff_val].u_val = atof(token);
								if (rrd.rra_def[rrd.stat_head->rra_cnt].
										par[RRA_cdp_xff_val].u_val < 0.0
										|| rrd.rra_def[rrd.stat_head->rra_cnt].
										par[RRA_cdp_xff_val].u_val >= 1.0)
									ret = -RRD_ERR_INVALID_XFF;
								break;
						}
						break;
					case 2:
						switch (cf_conv
								(rrd.rra_def[rrd.stat_head->rra_cnt].cf_nam)) {
							case CF_HWPREDICT:
							case CF_MHWPREDICT:
								rrd.rra_def[rrd.stat_head->rra_cnt].par[RRA_hw_alpha].
									u_val = atof(token);
								if (atof(token) <= 0.0 || atof(token) >= 1.0)
									ret = -RRD_ERR_INVALID_ALPHA;
								break;
							case CF_DEVSEASONAL:
							case CF_SEASONAL:
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_seasonal_gamma].u_val = atof(token);
								if (atof(token) <= 0.0 || atof(token) >= 1.0)
									ret = -RRD_ERR_INVALID_GAMMA;
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_seasonal_smooth_idx].u_cnt =
									hashed_name %
									rrd.rra_def[rrd.stat_head->rra_cnt].row_cnt;
								break;
							case CF_FAILURES:
								/* specifies the # of violations that constitutes the failure threshold */
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_failure_threshold].u_cnt = atoi(token);
								if (atoi(token) < 1
										|| atoi(token) > MAX_FAILURES_WINDOW_LEN)
									ret = -RRD_ERR_FAILURE_THRESHOLD_OUT_OF_RANGE;
								break;
							case CF_DEVPREDICT:
								/* specifies the index (1-based) of CF_DEVSEASONAL array
								 * associated with this CF_DEVPREDICT array. */
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_dependent_rra_idx].u_cnt =
									atoi(token) - 1;
								break;
							default:
								rrd.rra_def[rrd.stat_head->rra_cnt].pdp_cnt =
									atoi(token);
								if (atoi(token) < 1)
									ret = -RRD_ERR_INVALID_STEP;
								break;
						}
						break;
					case 3:
						switch (cf_conv(rrd.rra_def[rrd.stat_head->rra_cnt].cf_nam)) {
							case CF_HWPREDICT:
							case CF_MHWPREDICT:
								rrd.rra_def[rrd.stat_head->rra_cnt].par[RRA_hw_beta].
									u_val = atof(token);
								if (atof(token) < 0.0 || atof(token) > 1.0)
									ret = -RRD_ERR_INVALID_BETA;
								break;
							case CF_DEVSEASONAL:
							case CF_SEASONAL:
								/* specifies the index (1-based) of CF_HWPREDICT array
								 * associated with this CF_DEVSEASONAL or CF_SEASONAL array. 
								 * */
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_dependent_rra_idx].u_cnt =
									atoi(token) - 1;
								break;
							case CF_FAILURES:
								/* specifies the window length */
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_window_len].u_cnt = atoi(token);
								if (atoi(token) < 1
										|| atoi(token) > MAX_FAILURES_WINDOW_LEN)
									ret = RRD_ERR_WIN_LEN_OUT_OF_RANGE;
								/* verify that window length exceeds the failure threshold */
								if (rrd.rra_def[rrd.stat_head->rra_cnt].
										par[RRA_window_len].u_cnt <
										rrd.rra_def[rrd.stat_head->rra_cnt].
										par[RRA_failure_threshold].u_cnt)
									ret = -RRD_ERR_WINLEN_SHORTER_FAILURE_THRESHOLD;
								break;
							case CF_DEVPREDICT:
								/* shouldn't be any more arguments */
								ret = -RRD_ERR_INVALID_ARG1;
								break;
							default:
								row_cnt = atoi(token);
								if (row_cnt <= 0)
									ret = -RRD_ERR_INVALID_ROW_COUNT;
#if SIZEOF_TIME_T == 4
								if ((long long) pdp_step * rrd.rra_def[rrd.stat_head->rra_cnt].pdp_cnt * row_cnt > 4294967296LL){
									/* database timespan > 2**32, would overflow time_t */
									ret = -RRD_ERR_TIME_TOO_LARGE;
								}
#endif
								rrd.rra_def[rrd.stat_head->rra_cnt].row_cnt = row_cnt;
								break;
						}
						break;
					case 4:
						switch (cf_conv(rrd.rra_def[rrd.stat_head->rra_cnt].cf_nam)) {
							case CF_FAILURES:
								/* specifies the index (1-based) of CF_DEVSEASONAL array
								 * associated with this CF_DEVFAILURES array. */
								rrd.rra_def[rrd.stat_head->rra_cnt].
									par[RRA_dependent_rra_idx].u_cnt =
									atoi(token) - 1;
								break;
							case CF_DEVSEASONAL:
							case CF_SEASONAL:
								/* optional smoothing window */
								if (sscanf(token, "smoothing-window=%lf",
											&(rrd.rra_def[rrd.stat_head->rra_cnt].
												par[RRA_seasonal_smoothing_window].
												u_val))) {
									strcpy(rrd.stat_head->version, RRD_VERSION);    /* smoothing-window causes Version 4 */
									if (rrd.rra_def[rrd.stat_head->rra_cnt].
											par[RRA_seasonal_smoothing_window].u_val < 0.0
											|| rrd.rra_def[rrd.stat_head->rra_cnt].
											par[RRA_seasonal_smoothing_window].u_val >
											1.0) {
										ret = -RRD_ERR_INVALID_SMOOTHING_WINDOW;
									}
								} else {
									ret = -RRD_ERR_INVALID_OPT;
								}
								break;
							case CF_HWPREDICT:
							case CF_MHWPREDICT:
								/* length of the associated CF_SEASONAL and CF_DEVSEASONAL arrays. */
								period = atoi(token);
								if (period >
										rrd.rra_def[rrd.stat_head->rra_cnt].row_cnt)
									ret = -RRD_ERR_LEN_OF_SEASONAL_CYCLE;
								break;
							default:
								/* shouldn't be any more arguments */
								ret = -RRD_ERR_INVALID_ARG2;
								break;
						}
						break;
					case 5:
						/* If we are here, this must be a CF_HWPREDICT RRA.
						 * Specifies the index (1-based) of CF_SEASONAL array
						 * associated with this CF_HWPREDICT array. If this argument 
						 * is missing, then the CF_SEASONAL, CF_DEVSEASONAL, CF_DEVPREDICT,
						 * CF_FAILURES.
						 * arrays are created automatically. */
						rrd.rra_def[rrd.stat_head->rra_cnt].
							par[RRA_dependent_rra_idx].u_cnt = atoi(token) - 1;
						break;
					default:
						/* should never get here */
						ret = -RRD_ERR_UNKNOWN_ERROR;
						break;
				}       /* end switch */
				if (ret) {
					/* all errors are unrecoverable */
					free(argvcopy);
					rrd_free2(&rrd);
					return ret;
				}
				token = strtok_r(NULL, ":", &tokptr);
				token_idx++;
			}           /* end while */
			free(argvcopy);
			if (token_idx < token_min){
				rrd_free2(&rrd);
				return(-RRD_ERR_ARG3);
			}
#ifdef DEBUG
			fprintf(stderr,
					"Creating RRA CF: %s, dep idx %lu, current idx %lu\n",
					rrd.rra_def[rrd.stat_head->rra_cnt].cf_nam,
					rrd.rra_def[rrd.stat_head->rra_cnt].
					par[RRA_dependent_rra_idx].u_cnt, rrd.stat_head->rra_cnt);
#endif
			/* should we create CF_SEASONAL, CF_DEVSEASONAL, and CF_DEVPREDICT? */
			if ((cf_conv(rrd.rra_def[rrd.stat_head->rra_cnt].cf_nam) ==
						CF_HWPREDICT
						|| cf_conv(rrd.rra_def[rrd.stat_head->rra_cnt].cf_nam) ==
						CF_MHWPREDICT)
					&& rrd.rra_def[rrd.stat_head->rra_cnt].
					par[RRA_dependent_rra_idx].u_cnt == rrd.stat_head->rra_cnt) {
#ifdef DEBUG
				fprintf(stderr, "Creating HW contingent RRAs\n");
#endif
				if (create_hw_contingent_rras(&rrd, period, hashed_name) ==
						-1) {
					rrd_free2(&rrd);
					return (-RRD_ERR_CREATING_RRA);
				}
			}
			rrd.stat_head->rra_cnt++;
		} else {
			rrd_free2(&rrd);
			return (-RRD_ERR_ARG4);
		}
	}


	if (rrd.stat_head->rra_cnt < 1) {
		rrd_free2(&rrd);
		return (-RRD_ERR_ARG5);
	}

	if (rrd.stat_head->ds_cnt < 1) {
		rrd_free2(&rrd);
		return (-RRD_ERR_ARG6);
	}
	return rrd_create_fn(filename, &rrd);
}

int parseGENERIC_DS( const char *def, rrd_t *rrd, int ds_idx) {
	char      minstr[DS_NAM_SIZE], maxstr[DS_NAM_SIZE];
	char     *old_locale;
	int       ret = 0;

	/*
	   int temp;

	   temp = sscanf(def,"%lu:%18[^:]:%18[^:]", 
	   &(rrd -> ds_def[ds_idx].par[DS_mrhb_cnt].u_cnt),
	   minstr,maxstr);
	   */
	old_locale = setlocale(LC_NUMERIC, "C");
	if (sscanf(def, "%lu:%18[^:]:%18[^:]",
				&(rrd->ds_def[ds_idx].par[DS_mrhb_cnt].u_cnt),
				minstr, maxstr) == 3) {
		if (minstr[0] == 'U' && minstr[1] == 0)
			rrd->ds_def[ds_idx].par[DS_min_val].u_val = DNAN;
		else
			rrd->ds_def[ds_idx].par[DS_min_val].u_val = atof(minstr);

		if (maxstr[0] == 'U' && maxstr[1] == 0)
			rrd->ds_def[ds_idx].par[DS_max_val].u_val = DNAN;
		else
			rrd->ds_def[ds_idx].par[DS_max_val].u_val = atof(maxstr);

		if (!isnan(rrd->ds_def[ds_idx].par[DS_min_val].u_val) &&
				!isnan(rrd->ds_def[ds_idx].par[DS_max_val].u_val) &&
				rrd->ds_def[ds_idx].par[DS_min_val].u_val
				>= rrd->ds_def[ds_idx].par[DS_max_val].u_val) {
			ret = -RRD_ERR_ARG7;
			setlocale(LC_NUMERIC, old_locale);
			return ret;
		}
	} else {
		ret = -RRD_ERR_ARG8;
	}
	setlocale(LC_NUMERIC, old_locale);
	return ret;
}

/* Create the CF_DEVPREDICT, CF_DEVSEASONAL, CF_SEASONAL, and CF_FAILURES RRAs
 * associated with a CF_HWPREDICT RRA. */
int create_hw_contingent_rras(
		rrd_t *rrd,
		unsigned short period,
		unsigned long hashed_name)
{
	size_t    old_size;
	rra_def_t *current_rra;

	/* save index to CF_HWPREDICT */
	unsigned long hw_index = rrd->stat_head->rra_cnt;

	/* advance the pointer */
	(rrd->stat_head->rra_cnt)++;
	/* allocate the memory for the 4 contingent RRAs */
	old_size = sizeof(rra_def_t) * (rrd->stat_head->rra_cnt);
	if ((rrd->rra_def = (rra_def_t*)rrd_realloc(rrd->rra_def,
					old_size + 4 * sizeof(rra_def_t))) ==
			NULL) {
		rrd_free2(rrd);
		return (-RRD_ERR_ALLOC);
	}
	/* clear memory */
	memset(&(rrd->rra_def[rrd->stat_head->rra_cnt]), 0,
			4 * sizeof(rra_def_t));

	/* create the CF_SEASONAL RRA */
	current_rra = &(rrd->rra_def[rrd->stat_head->rra_cnt]);
	strcpy(current_rra->cf_nam, "SEASONAL");
	current_rra->row_cnt = period;
	current_rra->par[RRA_seasonal_smooth_idx].u_cnt = hashed_name % period;
	current_rra->pdp_cnt = 1;
	current_rra->par[RRA_seasonal_gamma].u_val =
		rrd->rra_def[hw_index].par[RRA_hw_alpha].u_val;
	current_rra->par[RRA_dependent_rra_idx].u_cnt = hw_index;
	rrd->rra_def[hw_index].par[RRA_dependent_rra_idx].u_cnt =
		rrd->stat_head->rra_cnt;

	/* create the CF_DEVSEASONAL RRA */
	(rrd->stat_head->rra_cnt)++;
	current_rra = &(rrd->rra_def[rrd->stat_head->rra_cnt]);
	strcpy(current_rra->cf_nam, "DEVSEASONAL");
	current_rra->row_cnt = period;
	current_rra->par[RRA_seasonal_smooth_idx].u_cnt = hashed_name % period;
	current_rra->pdp_cnt = 1;
	current_rra->par[RRA_seasonal_gamma].u_val =
		rrd->rra_def[hw_index].par[RRA_hw_alpha].u_val;
	current_rra->par[RRA_dependent_rra_idx].u_cnt = hw_index;

	/* create the CF_DEVPREDICT RRA */
	(rrd->stat_head->rra_cnt)++;
	current_rra = &(rrd->rra_def[rrd->stat_head->rra_cnt]);
	strcpy(current_rra->cf_nam, "DEVPREDICT");
	current_rra->row_cnt = (rrd->rra_def[hw_index]).row_cnt;
	current_rra->pdp_cnt = 1;
	current_rra->par[RRA_dependent_rra_idx].u_cnt = hw_index + 2;   /* DEVSEASONAL */

	/* create the CF_FAILURES RRA */
	(rrd->stat_head->rra_cnt)++;
	current_rra = &(rrd->rra_def[rrd->stat_head->rra_cnt]);
	strcpy(current_rra->cf_nam, "FAILURES");
	current_rra->row_cnt = period;
	current_rra->pdp_cnt = 1;
	current_rra->par[RRA_delta_pos].u_val = 2.0;
	current_rra->par[RRA_delta_neg].u_val = 2.0;
	current_rra->par[RRA_failure_threshold].u_cnt = 7;
	current_rra->par[RRA_window_len].u_cnt = 9;
	current_rra->par[RRA_dependent_rra_idx].u_cnt = hw_index + 2;   /* DEVSEASONAL */
	return 0;
}

/* create and empty rrd file according to the specs given */

int rrd_create_fn(
		const char *file_name,
		rrd_t *rrd)
{
	unsigned long i, ii;
	rrd_value_t *unknown;
	int       unkn_cnt;
	rrd_file_t *rrd_file_dn;
	rrd_t     rrd_dn;
	unsigned  rrd_flags = RRD_READWRITE | RRD_CREAT;
	int ret = 0;

	if (opt_no_overwrite) {
		rrd_flags |= RRD_EXCL ;
	}

	unkn_cnt = 0;
	for (i = 0; i < rrd->stat_head->rra_cnt; i++)
		unkn_cnt += rrd->stat_head->ds_cnt * rrd->rra_def[i].row_cnt;

	if ((rrd_file_dn = rrd_open(file_name, rrd, rrd_flags, &ret)) == NULL) {
		rrd_free2(rrd);
		return ret;
	}

	rrd_write(rrd_file_dn, rrd->stat_head, sizeof(stat_head_t));

	rrd_write(rrd_file_dn, rrd->ds_def, sizeof(ds_def_t) * rrd->stat_head->ds_cnt);

	rrd_write(rrd_file_dn, rrd->rra_def,
			sizeof(rra_def_t) * rrd->stat_head->rra_cnt);

	rrd_write(rrd_file_dn, rrd->live_head, sizeof(live_head_t));

	if ((rrd->pdp_prep = (pdp_prep_t*)calloc(1, sizeof(pdp_prep_t))) == NULL) {
		rrd_free2(rrd);
		rrd_close(rrd_file_dn);
		return (-RRD_ERR_ALLOC);
	}

	strcpy(rrd->pdp_prep->last_ds, "U");

	rrd->pdp_prep->scratch[PDP_val].u_val = 0.0;
	rrd->pdp_prep->scratch[PDP_unkn_sec_cnt].u_cnt =
		rrd->live_head->last_up % rrd->stat_head->pdp_step;

	for (i = 0; i < rrd->stat_head->ds_cnt; i++)
		rrd_write(rrd_file_dn, rrd->pdp_prep, sizeof(pdp_prep_t));

	if ((rrd->cdp_prep = (cdp_prep_t*)calloc(1, sizeof(cdp_prep_t))) == NULL) {
		rrd_free2(rrd);
		rrd_close(rrd_file_dn);
		return (-RRD_ERR_ALLOC);
	}


	for (i = 0; i < rrd->stat_head->rra_cnt; i++) {
		switch (cf_conv(rrd->rra_def[i].cf_nam)) {
			case CF_HWPREDICT:
			case CF_MHWPREDICT:
				init_hwpredict_cdp(rrd->cdp_prep);
				break;
			case CF_SEASONAL:
			case CF_DEVSEASONAL:
				init_seasonal_cdp(rrd->cdp_prep);
				break;
			case CF_FAILURES:
				/* initialize violation history to 0 */
				for (ii = 0; ii < MAX_CDP_PAR_EN; ii++) {
					/* We can zero everything out, by setting u_val to the
					 * NULL address. Each array entry in scratch is 8 bytes
					 * (a double), but u_cnt only accessed 4 bytes (long) */
					rrd->cdp_prep->scratch[ii].u_val = 0.0;
				}
				break;
			default:
				/* can not be zero because we don't know anything ... */
				rrd->cdp_prep->scratch[CDP_val].u_val = DNAN;
				/* startup missing pdp count */
				rrd->cdp_prep->scratch[CDP_unkn_pdp_cnt].u_cnt =
					((rrd->live_head->last_up -
					  rrd->pdp_prep->scratch[PDP_unkn_sec_cnt].u_cnt)
					 % (rrd->stat_head->pdp_step
						 * rrd->rra_def[i].pdp_cnt)) / rrd->stat_head->pdp_step;
				break;
		}

		for (ii = 0; ii < rrd->stat_head->ds_cnt; ii++) {
			rrd_write(rrd_file_dn, rrd->cdp_prep, sizeof(cdp_prep_t));
		}
	}

	/* now, we must make sure that the rest of the rrd
	   struct is properly initialized */

	if ((rrd->rra_ptr = (rra_ptr_t*)calloc(1, sizeof(rra_ptr_t))) == NULL) {
		rrd_free2(rrd);
		rrd_close(rrd_file_dn);
		return -RRD_ERR_ALLOC;
	}

	/* changed this initialization to be consistent with
	 * rrd_restore. With the old value (0), the first update
	 * would occur for cur_row = 1 because rrd_update increments
	 * the pointer a priori. */
	for (i = 0; i < rrd->stat_head->rra_cnt; i++) {
		rrd->rra_ptr->cur_row = rrd_select_initial_row(rrd_file_dn, i, &rrd->rra_def[i]);
		rrd_write(rrd_file_dn, rrd->rra_ptr, sizeof(rra_ptr_t));
	}

	/* write the empty data area */
	if ((unknown = (rrd_value_t *) malloc(512 * sizeof(rrd_value_t))) == NULL) {
		rrd_free2(rrd);
		rrd_close(rrd_file_dn);
		return -RRD_ERR_ALLOC;
	}
	for (i = 0; i < 512; ++i)
		unknown[i] = DNAN;

	while (unkn_cnt > 0) {
		if(rrd_write(rrd_file_dn, unknown, sizeof(rrd_value_t) * min(unkn_cnt, 512)) < 0)
		{
			return -RRD_ERR_CREATE_WRITE;
		}

		unkn_cnt -= 512;
	}
	free(unknown);
	rrd_free2(rrd);
	if (rrd_close(rrd_file_dn) == -1) {
		return -RRD_ERR_CREATE_WRITE;
	}
	/* flush all we don't need out of the cache */
	rrd_init(&rrd_dn);
	if((rrd_file_dn = rrd_open(file_name, &rrd_dn, RRD_READONLY, &ret)) != NULL)
	{
		rrd_dontneed(rrd_file_dn, &rrd_dn);
		/* rrd_free(&rrd_dn); */
		rrd_close(rrd_file_dn);
	}
	return ret;
}


static void rrd_free2(
		rrd_t *rrd)
{
	free(rrd->live_head);
	free(rrd->stat_head);
	free(rrd->ds_def);
	free(rrd->rra_def);
	free(rrd->rra_ptr);
	free(rrd->pdp_prep);
	free(rrd->cdp_prep);
	free(rrd->rrd_value);
}

