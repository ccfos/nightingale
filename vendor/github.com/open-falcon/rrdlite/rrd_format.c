/*****************************************************************************
 * RRDtool 1.4.9  Copyright by Tobi Oetiker, 1997-2014
 *****************************************************************************
 * rrd_format.c  RRD Database Format helper functions
 *****************************************************************************
 * $Id$
 * $Log$
 * Revision 1.5  2004/05/18 18:53:03  oetiker
 * big spell checking patch -- slif@bellsouth.net
 *
 * Revision 1.4  2003/02/13 07:05:27  oetiker
 * Find attached the patch I promised to send to you. Please note that there
 * are three new source files (src/rrd_is_thread_safe.h, src/rrd_thread_safe.c
 * and src/rrd_not_thread_safe.c) and the introduction of librrd_th. This
 * library is identical to librrd, but it contains support code for per-thread
 * global variables currently used for error information only. This is similar
 * to how errno per-thread variables are implemented.  librrd_th must be linked
 * alongside of libpthred
 *
 * There is also a new file "THREADS", holding some documentation.
 *
 * -- Peter Stamfest <peter@stamfest.at>
 *
 * Revision 1.3  2002/02/01 20:34:49  oetiker
 * fixed version number and date/time
 *
 * Revision 1.2  2001/03/10 23:54:39  oetiker
 * Support for COMPUTE data sources (CDEF data sources). Removes the RPN
 * parser and calculator from rrd_graph and puts then in a new file,
 * rrd_rpncalc.c. Changes to core files rrd_create and rrd_update. Some
 * clean-up of aberrant behavior stuff, including a bug fix.
 * Documentation update (rrdcreate.pod, rrdupdate.pod). Change xml format.
 * -- Jake Brutlag <jakeb@corp.webtv.net>
 *
 * Revision 1.1.1.1  2001/02/25 22:25:05  oetiker
 * checkin
 *
 * Revision 1.3  1998/03/08 12:35:11  oetiker
 * checkpointing things because the current setup seems to work
 * according to the things said in the manpages
 *
 * Revision 1.2  1998/02/26 22:58:22  oetiker
 * fixed define
 *
 * Revision 1.1  1998/02/21 16:14:41  oetiker
 * Initial revision
 *
 *
 *****************************************************************************/
#include "rrd_tool.h"
#ifdef WIN32
#include "stdlib.h"
#endif

#define converter(VV,VVV) \
	if (strcmp(#VV, string) == 0) return VVV;

/* conversion functions to allow symbolic entry of enumerations */
enum dst_en dst_conv( char *string) {
	converter(COUNTER, DST_COUNTER)
		converter(ABSOLUTE, DST_ABSOLUTE)
		converter(GAUGE, DST_GAUGE)
		converter(DERIVE, DST_DERIVE)
		converter(COMPUTE, DST_CDEF)
		return (enum dst_en)(-1);
}


enum cf_en cf_conv( const char *string) {

	converter(AVERAGE, CF_AVERAGE)
		converter(MIN, CF_MINIMUM)
		converter(MAX, CF_MAXIMUM)
		converter(LAST, CF_LAST)
		converter(HWPREDICT, CF_HWPREDICT)
		converter(MHWPREDICT, CF_MHWPREDICT)
		converter(DEVPREDICT, CF_DEVPREDICT)
		converter(SEASONAL, CF_SEASONAL)
		converter(DEVSEASONAL, CF_DEVSEASONAL)
		converter(FAILURES, CF_FAILURES)
		return (enum cf_en)(-1);
}

#undef converter

long ds_match( rrd_t *rrd, char *ds_nam) {
	unsigned long i;

	for (i = 0; i < rrd->stat_head->ds_cnt; i++)
		if ((strcmp(ds_nam, rrd->ds_def[i].ds_nam)) == 0)
			return i;
	return -RRD_ERR_UNKNOWN_DS_NAME;
}

off_t rrd_get_header_size( rrd_t *rrd) {
	return sizeof(stat_head_t) + \
		sizeof(ds_def_t) * rrd->stat_head->ds_cnt + \
		sizeof(rra_def_t) * rrd->stat_head->rra_cnt + \
		( atoi(rrd->stat_head->version) < 3 ? sizeof(time_t) : sizeof(live_head_t) ) + \
		sizeof(pdp_prep_t) * rrd->stat_head->ds_cnt + \
		sizeof(cdp_prep_t) * rrd->stat_head->ds_cnt * rrd->stat_head->rra_cnt + \
		sizeof(rra_ptr_t) * rrd->stat_head->rra_cnt;
}
