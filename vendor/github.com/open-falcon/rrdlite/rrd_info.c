/*****************************************************************************
 * RRDtool 1.4.9  Copyright by Tobi Oetiker, 1997-2014
 *****************************************************************************
 * rrd_info  Get Information about the configuration of an RRD
 *****************************************************************************/

#include "rrd_tool.h"
#include "rrd_rpncalc.h"
#ifndef RRD_LITE
#include "rrd_client.h"
#endif
#include <stdarg.h>

/* proto */
rrd_info_t *rrd_info(int, char **);
rrd_info_t *rrd_info_r(char *filename, int *ret_p);

/* allocate memory for string */
char *sprintf_alloc(char *fmt, ...) {
	char     *str = NULL;
	va_list   argp;
#ifdef HAVE_VASPRINTF
	va_start( argp, fmt );
	if (vasprintf( &str, fmt, argp ) == -1){
		va_end(argp);
		//rrd_set_error ("vasprintf failed.");
		return(NULL);
	}
#else
	int       maxlen = 1024 + strlen(fmt);
	str = (char*)malloc(sizeof(char) * (maxlen + 1));
	if (str != NULL) {
		va_start(argp, fmt);
#ifdef HAVE_VSNPRINTF
		vsnprintf(str, maxlen, fmt, argp);
#else
		vsprintf(str, fmt, argp);
#endif
	}
#endif /* HAVE_VASPRINTF */
	va_end(argp);
	return str;
}

/* the function formerly known as push was renamed to info_push and later
 * rrd_info_push because it is now used outside the scope of this file */
rrd_info_t * rrd_info_push(rrd_info_t * info, char *key, rrd_info_type_t type, 
		rrd_infoval_t value) {
	rrd_info_t *next;

	next = (rrd_info_t*)malloc(sizeof(*next));
	next->next = (rrd_info_t *) 0;
	if (info)
		info->next = next;
	next->type = type;
	next->key = key;
	switch (type) {
		case RD_I_VAL:
			next->value.u_val = value.u_val;
			break;
		case RD_I_CNT:
			next->value.u_cnt = value.u_cnt;
			break;
		case RD_I_INT:
			next->value.u_int = value.u_int;
			break;
		case RD_I_STR:
			next->value.u_str = (char*)malloc(sizeof(char) * (strlen(value.u_str) + 1));
			strcpy(next->value.u_str, value.u_str);
			break;
		case RD_I_BLO:
			next->value.u_blo.size = value.u_blo.size;
			next->value.u_blo.ptr =
				(unsigned char *)malloc(sizeof(unsigned char) * value.u_blo.size);
			memcpy(next->value.u_blo.ptr, value.u_blo.ptr, value.u_blo.size);
			break;
	}
	return (next);
}

rrd_info_t *rrd_info_r(char *filename, int *ret_p) {
	unsigned int i, ii = 0;
	rrd_t     rrd;
	rrd_info_t *data = NULL, *cd;
	rrd_infoval_t info;
	rrd_file_t *rrd_file;
	enum cf_en current_cf;
	enum dst_en current_ds;

	rrd_init(&rrd);
	rrd_file = rrd_open(filename, &rrd, RRD_READONLY, ret_p);
	if (rrd_file == NULL)
		goto err_free;

	info.u_str = filename;
	cd = rrd_info_push(NULL, sprintf_alloc("filename"), RD_I_STR, info);
	data = cd;

	info.u_str = rrd.stat_head->version;
	cd = rrd_info_push(cd, sprintf_alloc("rrd_version"), RD_I_STR, info);

	info.u_cnt = rrd.stat_head->pdp_step;
	cd = rrd_info_push(cd, sprintf_alloc("step"), RD_I_CNT, info);

	info.u_cnt = rrd.live_head->last_up;
	cd = rrd_info_push(cd, sprintf_alloc("last_update"), RD_I_CNT, info);

	info.u_cnt = rrd_get_header_size(&rrd);
	cd = rrd_info_push(cd, sprintf_alloc("header_size"), RD_I_CNT, info);

	for (i = 0; i < rrd.stat_head->ds_cnt; i++) {

		info.u_cnt=i;
		cd= rrd_info_push(cd,sprintf_alloc("ds[%s].index",
					rrd.ds_def[i].ds_nam),
				RD_I_CNT, info);

		info.u_str = rrd.ds_def[i].dst;
		cd = rrd_info_push(cd, sprintf_alloc("ds[%s].type",
					rrd.ds_def[i].ds_nam),
				RD_I_STR, info);

		current_ds = dst_conv(rrd.ds_def[i].dst);
		switch (current_ds) {
			case DST_CDEF:
				{
					char     *buffer = NULL;

					rpn_compact2str((rpn_cdefds_t *) &(rrd.ds_def[i].par[DS_cdef]),
							rrd.ds_def, &buffer);
					info.u_str = buffer;
					cd = rrd_info_push(cd,
							sprintf_alloc("ds[%s].cdef",
								rrd.ds_def[i].ds_nam), RD_I_STR,
							info);
					free(buffer);
				}
				break;
			default:
				info.u_cnt = rrd.ds_def[i].par[DS_mrhb_cnt].u_cnt;
				cd = rrd_info_push(cd,
						sprintf_alloc("ds[%s].minimal_heartbeat",
							rrd.ds_def[i].ds_nam), RD_I_CNT,
						info);

				info.u_val = rrd.ds_def[i].par[DS_min_val].u_val;
				cd = rrd_info_push(cd,
						sprintf_alloc("ds[%s].min",
							rrd.ds_def[i].ds_nam), RD_I_VAL,
						info);

				info.u_val = rrd.ds_def[i].par[DS_max_val].u_val;
				cd = rrd_info_push(cd,
						sprintf_alloc("ds[%s].max",
							rrd.ds_def[i].ds_nam), RD_I_VAL,
						info);
				break;
		}

		info.u_str = rrd.pdp_prep[i].last_ds;
		cd = rrd_info_push(cd,
				sprintf_alloc("ds[%s].last_ds",
					rrd.ds_def[i].ds_nam), RD_I_STR,
				info);

		info.u_val = rrd.pdp_prep[i].scratch[PDP_val].u_val;
		cd = rrd_info_push(cd,
				sprintf_alloc("ds[%s].value",
					rrd.ds_def[i].ds_nam), RD_I_VAL,
				info);

		info.u_cnt = rrd.pdp_prep[i].scratch[PDP_unkn_sec_cnt].u_cnt;
		cd = rrd_info_push(cd,
				sprintf_alloc("ds[%s].unknown_sec",
					rrd.ds_def[i].ds_nam), RD_I_CNT,
				info);
	}

	for (i = 0; i < rrd.stat_head->rra_cnt; i++) {
		info.u_str = rrd.rra_def[i].cf_nam;
		cd = rrd_info_push(cd, sprintf_alloc("rra[%d].cf", i), RD_I_STR,
				info);
		current_cf = cf_conv(rrd.rra_def[i].cf_nam);

		info.u_cnt = rrd.rra_def[i].row_cnt;
		cd = rrd_info_push(cd, sprintf_alloc("rra[%d].rows", i), RD_I_CNT,
				info);

		info.u_cnt = rrd.rra_ptr[i].cur_row;
		cd = rrd_info_push(cd, sprintf_alloc("rra[%d].cur_row", i), RD_I_CNT,
				info);

		info.u_cnt = rrd.rra_def[i].pdp_cnt;
		cd = rrd_info_push(cd, sprintf_alloc("rra[%d].pdp_per_row", i),
				RD_I_CNT, info);

		switch (current_cf) {
			case CF_HWPREDICT:
			case CF_MHWPREDICT:
				info.u_val = rrd.rra_def[i].par[RRA_hw_alpha].u_val;
				cd = rrd_info_push(cd, sprintf_alloc("rra[%d].alpha", i),
						RD_I_VAL, info);
				info.u_val = rrd.rra_def[i].par[RRA_hw_beta].u_val;
				cd = rrd_info_push(cd, sprintf_alloc("rra[%d].beta", i), RD_I_VAL,
						info);
				break;
			case CF_SEASONAL:
			case CF_DEVSEASONAL:
				info.u_val = rrd.rra_def[i].par[RRA_seasonal_gamma].u_val;
				cd = rrd_info_push(cd, sprintf_alloc("rra[%d].gamma", i),
						RD_I_VAL, info);
				if (atoi(rrd.stat_head->version) >= 4) {
					info.u_val =
						rrd.rra_def[i].par[RRA_seasonal_smoothing_window].u_val;
					cd = rrd_info_push(cd,
							sprintf_alloc("rra[%d].smoothing_window",
								i), RD_I_VAL, info);
				}
				break;
			case CF_FAILURES:
				info.u_val = rrd.rra_def[i].par[RRA_delta_pos].u_val;
				cd = rrd_info_push(cd, sprintf_alloc("rra[%d].delta_pos", i),
						RD_I_VAL, info);
				info.u_val = rrd.rra_def[i].par[RRA_delta_neg].u_val;
				cd = rrd_info_push(cd, sprintf_alloc("rra[%d].delta_neg", i),
						RD_I_VAL, info);
				info.u_cnt = rrd.rra_def[i].par[RRA_failure_threshold].u_cnt;
				cd = rrd_info_push(cd,
						sprintf_alloc("rra[%d].failure_threshold", i),
						RD_I_CNT, info);
				info.u_cnt = rrd.rra_def[i].par[RRA_window_len].u_cnt;
				cd = rrd_info_push(cd, sprintf_alloc("rra[%d].window_length", i),
						RD_I_CNT, info);
				break;
			case CF_DEVPREDICT:
				break;
			default:
				info.u_val = rrd.rra_def[i].par[RRA_cdp_xff_val].u_val;
				cd = rrd_info_push(cd, sprintf_alloc("rra[%d].xff", i), RD_I_VAL,
						info);
				break;
		}

		for (ii = 0; ii < rrd.stat_head->ds_cnt; ii++) {
			switch (current_cf) {
				case CF_HWPREDICT:
				case CF_MHWPREDICT:
					info.u_val =
						rrd.cdp_prep[i * rrd.stat_head->ds_cnt +
						ii].scratch[CDP_hw_intercept].u_val;
					cd = rrd_info_push(cd,
							sprintf_alloc
							("rra[%d].cdp_prep[%d].intercept", i, ii),
							RD_I_VAL, info);
					info.u_val =
						rrd.cdp_prep[i * rrd.stat_head->ds_cnt +
						ii].scratch[CDP_hw_slope].u_val;
					cd = rrd_info_push(cd,
							sprintf_alloc("rra[%d].cdp_prep[%d].slope",
								i, ii), RD_I_VAL, info);
					info.u_cnt =
						rrd.cdp_prep[i * rrd.stat_head->ds_cnt +
						ii].scratch[CDP_null_count].u_cnt;
					cd = rrd_info_push(cd,
							sprintf_alloc
							("rra[%d].cdp_prep[%d].NaN_count", i, ii),
							RD_I_CNT, info);
					break;
				case CF_SEASONAL:
					info.u_val =
						rrd.cdp_prep[i * rrd.stat_head->ds_cnt +
						ii].scratch[CDP_hw_seasonal].u_val;
					cd = rrd_info_push(cd,
							sprintf_alloc
							("rra[%d].cdp_prep[%d].seasonal", i, ii),
							RD_I_VAL, info);
					break;
				case CF_DEVSEASONAL:
					info.u_val =
						rrd.cdp_prep[i * rrd.stat_head->ds_cnt +
						ii].scratch[CDP_seasonal_deviation].u_val;
					cd = rrd_info_push(cd,
							sprintf_alloc
							("rra[%d].cdp_prep[%d].deviation", i, ii),
							RD_I_VAL, info);
					break;
				case CF_DEVPREDICT:
					break;
				case CF_FAILURES:
					{
						unsigned short j;
						char     *violations_array;
						char      history[MAX_FAILURES_WINDOW_LEN + 1];

						violations_array =
							(char *) rrd.cdp_prep[i * rrd.stat_head->ds_cnt +
							ii].scratch;
						for (j = 0; j < rrd.rra_def[i].par[RRA_window_len].u_cnt; ++j)
							history[j] = (violations_array[j] == 1) ? '1' : '0';
						history[j] = '\0';
						info.u_str = history;
						cd = rrd_info_push(cd,
								sprintf_alloc
								("rra[%d].cdp_prep[%d].history", i, ii),
								RD_I_STR, info);
					}
					break;
				default:
					info.u_val =
						rrd.cdp_prep[i * rrd.stat_head->ds_cnt +
						ii].scratch[CDP_val].u_val;
					cd = rrd_info_push(cd,
							sprintf_alloc("rra[%d].cdp_prep[%d].value",
								i, ii), RD_I_VAL, info);
					info.u_cnt =
						rrd.cdp_prep[i * rrd.stat_head->ds_cnt +
						ii].scratch[CDP_unkn_pdp_cnt].u_cnt;
					cd = rrd_info_push(cd,
							sprintf_alloc
							("rra[%d].cdp_prep[%d].unknown_datapoints",
							 i, ii), RD_I_CNT, info);
					break;
			}
		}
	}

	rrd_close(rrd_file);
err_free:
	rrd_free(&rrd);
	return (data);
}


void rrd_info_print(rrd_info_t * data) {
	while (data) {
		printf("%s = ", data->key);

		switch (data->type) {
			case RD_I_VAL:
				if (isnan(data->value.u_val))
					printf("NaN\n");
				else
					printf("%0.10e\n", data->value.u_val);
				break;
			case RD_I_CNT:
				printf("%lu\n", data->value.u_cnt);
				break;
			case RD_I_INT:
				printf("%d\n", data->value.u_int);
				break;
			case RD_I_STR:
				printf("\"%s\"\n", data->value.u_str);
				break;
			case RD_I_BLO:
				printf("BLOB_SIZE:%lu\n", data->value.u_blo.size);
				fwrite(data->value.u_blo.ptr, data->value.u_blo.size, 1, stdout);
				break;
		}
		data = data->next;
	}
}

void rrd_info_free(rrd_info_t * data) {
	rrd_info_t *save;

	while (data) {
		save = data;
		if (data->key) {
			if (data->type == RD_I_STR) {
				free(data->value.u_str);
			}
			if (data->type == RD_I_BLO) {
				free(data->value.u_blo.ptr);
			}
			free(data->key);
		}
		data = data->next;
		free(save);
	}
}
