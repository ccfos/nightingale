/*****************************************************************************
 * RRDtool 1.4.9  Copyright by Tobi Oetiker, 1997-2014
 *****************************************************************************
 * rrd_error.c   Common Header File
 *****************************************************************************
 * $Id$
 * $Log$
 * Revision 1.4  2003/02/22 21:57:03  oetiker
 * a patch to avoid a memory leak and a Makefile.am patch to
 * distribute all required source files -- Peter Stamfest <peter@stamfest.at>
 *
 * Revision 1.3  2003/02/13 07:05:27  oetiker
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
 * Revision 1.2  2002/02/01 20:34:49  oetiker
 * fixed version number and date/time
 *
 * Revision 1.1.1.1  2001/02/25 22:25:05  oetiker
 * checkin
 *
 * changed by yubo@yubo.org
 *
 *************************************************************************** */


#include <stdlib.h>
#include <stdio.h>

#include "rrd_error.h"

char const *rrd_err_text[RRD_ERR_NUM] = {
	"allocating error", /* RRD_ERR_ALLOC */
	"Invalid DS name", /* RRD_ERR_INVALID_DS_NAME */
	"Invalid DS type", /* RRD_ERR_INVALID_DS_TYPE */
	"Duplicate DS name", /* RRD_ERR_DUPLICATE_DS_NAME */
	"Invalid DS format", /* RRD_ERR_INVALID_DS_FORMAT */
	"Invalid DS type specified", /* RRD_ERR_INVALID_DS_TYPE_SPEC */
	"creating rrd error", /* RRD_ERR_CREATE_WRITE */
	"Failed to parse CF name", /* RRD_ERR_FAILED_PARSE_CF_NAME */
	"Unrecognized consolidation function", /* RRD_ERR_UNREC_CONSOLIDATION_FUNC */
	"Invalid row count", /* RRD_ERR_INVALID_ROW_COUNT */
	"Invalid xff: must be between 0 and 1", /* RRD_ERR_INVALID_XFF */
	"Invalid alpha: must be between 0 and 1", /* RRD_ERR_INVALID_ALPHA */
	"Invalid gamma: must be between 0 and 1", /* RRD_ERR_INVALID_GAMMA */
	"Failure threshold is out of range 1, 28", /* RRD_ERR_FAILURE_THRESHOLD_OUT_OF_RANGE */
	"Invalid step: must be >= 1", /* RRD_ERR_INVALID_STEP */
	"Invalid beta: must be between 0 and 1", /* RRD_ERR_INVALID_BETA */
	"Window length is out of range 1, 28", /* RRD_ERR_WIN_LEN_OUT_OF_RANGE */
	"Window length is shorter than the failure threshold", /* RRD_ERR_WINLEN_SHORTER_FAILURE_THRESHOLD */
	"Unexpected extra argument for consolidation function DEVPREDICT", /* RRD_ERR_INVALID_ARG1 */
	"The time spanned by the database is too large: must be <= 4294967296 seconds", /* RRD_ERR_TIME_TOO_LARGE */
	"Invalid smoothing-window : must be between 0 and 1", /* RRD_ERR_INVALID_SMOOTHING_WINDOW */
	"Invalid option", /* RRD_ERR_INVALID_OPT */
	"Length of seasonal cycle exceeds length of HW prediction array", /* RRD_ERR_LEN_OF_SEASONAL_CYCLE */
	"Unexpected extra argument for consolidation function", /* RRD_ERR_INVALID_ARG2 */
	"Unknown error", /* RRD_ERR_UNKNOWN_ERROR */
	"Expected at least xxx arguments for RRA but got ooo", /* RRD_ERR_ARG3 */
	"creating contingent RRA", /* RRD_ERR_CREATING_RRA */
	"can't parse argument", /* RRD_ERR_ARG4 */
	"you must define at least one Round Robin Archive", /* RRD_ERR_ARG5 */
	"you must define at least one Data Source", /* RRD_ERR_ARG6 */
	"min must be less than max in DS definition", /* RRD_ERR_ARG7 */
	"failed to parse data source ??", /* RRD_ERR_ARG8 */
	"rrd_open() creating file error", /* RRD_ERR_CREATE_FILE1 */
	"malloc fetch ds_namv array", /* RRD_ERR_MALLOC1 */
	"malloc fetch ds_namv entry", /* RRD_ERR_MALLOC2 */
	"the RRD does not contain an RRA matching the chosen CF", /* RRD_ERR_NO_MATCH_RRA */
	"malloc fetch data area", /* RRD_ERR_MALLOC3 */
	"seek error in RRA", /* RRD_ERR_SEEK_RRA */
	"wrap seek in RRA did fail", /*  RRD_ERR_SEEK_RRA1 */
	"fetching cdp from rra", /* RRD_ERR_FETCH_CDP */
	"unknown data source name", /* RRD_ERR_UNKNOWN_DS_NAME */
	"memory allocation failure: seasonal coef", /* RRD_ERR_MALLOC4 */
	"read operation failed in lookup_seasonal()", /* RRD_ERR_READ1 */
	"seek operation failed in lookup_seasonal()", /* RRD_ERR_SEEK1 */
	"apply smoother: memory allocation failure", /* RRD_ERR_MALLOC5 */
	"seek to rra failed", /* RRD_ERR_SEEK2 */
	"reading value failed: ??", /* RRD_ERR_READ2 */
	"apply smoother: SEASONAL rra doesn't have valid dependency", /* RRD_ERR_DEP1 */
	"apply_smoother: seek to cdp_prep failed", /* RRD_ERR_SEEK3 */
	"apply_smoother: cdp_prep write failed", /* RRD_ERR_WRITE1 */
	"apply_smoother: seek to pos ?? failed", /* RRD_ERR_SEEK4 */
	"apply_smoother: write failed to xxx", /* RRD_ERR_WRITE2 */
	"reached EOF while loading header ", /* RRD_ERR_READ3 */
	"rrd_read() malloc error", /* RRD_ERR_MALLOC6 */
	"short read while reading header ", /* RRD_ERR_READ4 */
	"allocating rrd_file descriptor for 'xxx'", /* RRD_ERR_MALLOC7 */
	"allocating rrd_simple_file for 'xxx'", /* RRD_ERR_MALLOC8 */
	"in read/write request mask", /* RRD_ERR_IO1 */
	"opening error", /* RRD_ERR_OPEN_FILE */
	"fstat error", /* RRD_ERR_STAT_FILE */
	"write error", /* RRD_ERR_WRITE5 */
	"mmap error", /* RRD_ERR_MMAP */
	"This file is not an RRD file", /* RRD_ERR_FILE */
	"This RRD was created on another architecture", /* RRD_ERR_FILE1 */
	"can't handle RRD file version", /* RRD_ERR_FILE2 */
	"live_head_t malloc", /* RRD_ERR_MALLOC9 */
	"file is too small (should be ?? bytes)", /* RRD_ERR_FILE3 */
	"msync rrd_file error", /* RRD_ERR_MSYNC */
	"munmap rrd_file error", /* RRD_ERR_MUNMAP */
	"closing rrd_file error", /* RRD_ERR_CLOSE */
	"attempting to write beyond end of file", /* RRD_ERR_WRITE6 */
	"update process_arg error", /* RRD_ERR_ARG9 */
	"write changes to disk error", /* RRD_ERR_WRITE7 */
	"Not enough arguments", /* RRD_ERR_ARG10 */
	"could not lock RRD", /* RRD_ERR_LOCK */
	"failed duplication argv entry", /* RRD_ERR_FAILED_STRDUP */
	"allocating updvals pointer array.", /* RRD_ERR_MALLOC10 */
	"allocating pdp_temp.", /* RRD_ERR_MALLOC11 */
	"allocating skip_update.", /* RRD_ERR_MALLOC12 */
	"allocating tmpl_idx.", /* RRD_ERR_MALLOC13 */
	"allocating rra_step_cnt.", /* RRD_ERR_MALLOC14 */
	"allocating pdp_new.", /* RRD_ERR_MALLOC15 */
	"parse template error", /* RRD_ERR_PARSE */
	"error copying tmplt ", /* RRD_ERR_FAILED_STRDUP1 */
	"tmplt contains more DS definitions than RRD", /* RRD_ERR_MORE_DS */
	"unknown DS name ", /* RRD_ERR_UNKNOWN_DS_NAME1 */
	"expected timestamp not found in data source from ??", /* RRD_ERR_STR */
	"found extra data on update argument: ??", /* RRD_ERR_ARG11 */
	"expected ?? data source readings (got ??) from ??", /* RRD_ERR_EXPECTED */
	"ds time: ??: ??", /* RRD_ERR_TIME1 */
	"specifying time relative to the 'start' or 'end' makes no sense here: ??", /* RRD_ERR_TIME2 */
	"strtod error: converting ?? to float: ??", /* RRD_ERR_STRTOD */
	"illegal attempt to update using time ?? when last update time is ?? (minimum one second step)", /* RRD_ERR_TIME3 */
	"not a simple ?? integer: '??'", /* RRD_ERR_INT */
	"conversion of '??' to float not complete: tail '??'", /* RRD_ERR_DATA */
	"rrd contains unknown DS type : '??'", /* RRD_ERR_UNKNOWN_DS_TYPE */
	"seek error in rrd", /* RRD_ERR_SEEK5 */
	"writing rrd: ??", /* RRD_ERR_WRITE8 */
	"seek rrd for live header writeback", /* RRD_ERR_SEEK6 */
	"rrd_write live_head to rrd", /* RRD_ERR_WRITE9 */
	"rrd_write pdp_prep to rrd", /* RRD_ERR_WRITE10 */
	"rrd_write cdp_prep to rrd", /* RRD_ERR_WRITE11 */
	"rrd_write rra_ptr to rrd", /* RRD_ERR_WRITE12 */
	"the start and end times cannot be specified relative to each other", /* RRD_ERR_TIME4 */
	"the start time cannot be specified relative to itself", /* RRD_ERR_TIME5 */
	"the end time cannot be specified relative to itself", /* RRD_ERR_TIME6 */
	"failed to alloc memory in addop2str", /* RRD_ERR_MALLOC16 */
	"failed to parse computed data source", /* RRD_ERR_PARSE1 */
	"operators TIME, LTIME, PREV COUNT TREND TRENDNAN PREDICT PREDICTSIGMA are not supported with DS COMPUTE", /* RRD_ERR_DS */
	"don't undestand expr", /* RRD_ERR_EXPR */
	"RPN stack overflow", /* RRD_ERR_STACK */
	"RPN stack underflow", /* RRD_ERR_STACK1 */
	"VDEF made it into rpn_calc... aborting", /* RRD_ERR_ABORT */
	"negative shift step not allowed: ??", /* RRD_ERR_ALLOW */
	"malformed trend arguments", /* RRD_ERR_ARG12 */
	"RPN final stack size != 1", /* RRD_ERR_STACK2 */
	"Maximum ?? RPN nodes permitted. Got ?? RPN nodes at present.", /* RRD_ERR_DATA1 */
	"constants must be integers in the interval (??, ??)", /* RRD_ERR_DATA2 */
	"failed allocating rpnp array", /* RRD_ERR_MALLOC17 */
	"unknown data acquisition function '??'", /* RRD_ERR_UNKNOWN_DATA */
	"update_cdp_prep error", /* RRD_ERR_UPDATE_CDP */
	"variable '??' not found", /* RRD_ERR_UNKNOWN_DATA1 */
};



const char *rrd_strerror(int err) {
	int e;
	e = abs(err);
	if(e == 0){
		return NULL;
	}else{
		if (e >= RRD_ERR_START &&  e <= RRD_ERR_END){
			printf("errno: 0x%04x, str:%s\n", e, rrd_err_text[e-RRD_ERR_START]);
			return rrd_err_text[e-RRD_ERR_START];
		}else{
			printf("errno: 0x%04x, str:%s\n", e, rrd_err_text[RRD_ERR_UNKNOWN_ERROR-RRD_ERR_START]);
			return rrd_err_text[RRD_ERR_UNKNOWN_ERROR-RRD_ERR_START];
		}
	}
}



