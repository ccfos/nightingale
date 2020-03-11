/*****************************************************************************
 * RRDtool 1.4.9  Copyright by Tobi Oetiker, 1997-2014
 *****************************************************************************
 * rrd_format.h  RRD Database Format header
 *****************************************************************************/

#ifndef _RRD_FORMAT_H
#define _RRD_FORMAT_H

/* 
 * _RRD_TOOL_H
 *   We're building RRDTool itself.
 *
 * RRD_EXPORT_DEPRECATED
 *   User is requesting internal function which need this struct. They have
 *   been told that this will change and have agreed to adapt their programs.
 */
#if !defined(_RRD_TOOL_H) && !defined(RRD_EXPORT_DEPRECATED)
# error "Do not include rrd_format.h directly. Include rrd.h instead!"
#endif

#include "rrd.h"

/*****************************************************************************
 * put this in your /usr/lib/magic file (/etc/magic on HPUX)
 *
 *  # rrd database format
 *  0       string          RRD\0           rrd file
 *  >5      string          >\0             version '%s'
 *
 *****************************************************************************/

#define RRD_COOKIE    "RRD"
/* #define RRD_VERSION   "0002" */
/* changed because microsecond precision requires another field */
#define RRD_VERSION   "0004"
#define RRD_VERSION3  "0003"
#define FLOAT_COOKIE  ((double)8.642135E130)

typedef union unival {
    unsigned long u_cnt;
    rrd_value_t u_val;
} unival;


/****************************************************************************
 * The RRD Database Structure
 * ---------------------------
 * 
 * In oder to properly describe the database structure lets define a few
 * new words:
 *
 * ds - Data Source (ds) providing input to the database. A Data Source (ds)
 *       can be a traffic counter, a temperature, the number of users logged
 *       into a system. The rrd database format can handle the input of
 *       several Data Sources (ds) in a singe database.
 *  
 * dst - Data Source Type (dst). The Data Source Type (dst) defines the rules
 *       applied to Build Primary Data Points from the input provided by the
 *       data sources (ds).
 *
 * pdp - Primary Data Point (pdp). After the database has accepted the
 *       input from the data sources (ds). It starts building Primary
 *       Data Points (pdp) from the data. Primary Data Points (pdp)
 *       are evenly spaced along the time axis (pdp_step). The values
 *       of the Primary Data Points are calculated from the values of
 *       the data source (ds) and the exact time these values were
 *       provided by the data source (ds).
 *
 * pdp_st - PDP Start (pdp_st). The moments (pdp_st) in time where
 *       these steps occur are defined by the moments where the
 *       number of seconds since 1970-jan-1 modulo pdp_step equals
 *       zero (pdp_st). 
 *
 * cf -  Consolidation Function (cf). An arbitrary Consolidation Function (cf)
 *       (averaging, min, max) is applied to the primary data points (pdp) to
 *       calculate the consolidated data point.
 *
 * cdp - Consolidated Data Point (cdp) is the long term storage format for data
 *       in the rrd database. Consolidated Data Points represent one or
 *       several primary data points collected along the time axis. The
 *       Consolidated Data Points (cdp) are stored in Round Robin Archives
 *       (rra).
 *
 * rra - Round Robin Archive (rra). This is the place where the
 *       consolidated data points (cdp) get stored. The data is
 *       organized in rows (row) and columns (col). The Round Robin
 *       Archive got its name from the method data is stored in
 *       there. An RRD database can contain several Round Robin
 *       Archives. Each Round Robin Archive can have a different row
 *       spacing along the time axis (pdp_cnt) and a different
 *       consolidation function (cf) used to build its consolidated
 *       data points (cdp).  
 * 
 * rra_st - RRA Start (rra_st). The moments (rra_st) in time where
 *       Consolidated Data Points (cdp) are added to an rra are
 *       defined by the moments where the number of seconds since
 *       1970-jan-1 modulo pdp_cnt*pdp_step equals zero (rra_st).
 *
 * row - Row (row). A row represent all consolidated data points (cdp)
 *       in a round robin archive who are of the same age.
 *       
 * col - Column (col). A column (col) represent all consolidated
 *       data points (cdp) in a round robin archive (rra) who
 *       originated from the same data source (ds).
 *
 */

/****************************************************************************
 * POS 1: stat_head_t                           static header of the database
 ****************************************************************************/

typedef struct stat_head_t {

    /* Data Base Identification Section ** */
    char      cookie[4];    /* RRD */
    char      version[5];   /* version of the format */
    double    float_cookie; /* is it the correct double
                             * representation ?  */

    /* Data Base Structure Definition **** */
    unsigned long ds_cnt;   /* how many different ds provide
                             * input to the rrd */
    unsigned long rra_cnt;  /* how many rras will be maintained
                             * in the rrd */
    unsigned long pdp_step; /* pdp interval in seconds */

    unival    par[10];  /* global parameters ... unused
                           at the moment */
} stat_head_t;


/****************************************************************************
 * POS 2: ds_def_t  (* ds_cnt)                        Data Source definitions
 ****************************************************************************/

enum dst_en { DST_COUNTER = 0,  /* data source types available */
    DST_ABSOLUTE,
    DST_GAUGE,
    DST_DERIVE,
    DST_CDEF
};

enum ds_param_en { DS_mrhb_cnt = 0, /* minimum required heartbeat. A
                                     * data source must provide input at
                                     * least every ds_mrhb seconds,
                                     * otherwise it is regarded dead and
                                     * will be set to UNKNOWN */
    DS_min_val,         /* the processed input of a ds must */
    DS_max_val,         /* be between max_val and min_val
                         * both can be set to UNKNOWN if you
                         * do not care. Data outside the limits
                         * set to UNKNOWN */
    DS_cdef = DS_mrhb_cnt
};                      /* pointer to encoded rpn
                         * expression only applies to DST_CDEF */

/* The magic number here is one less than DS_NAM_SIZE */
#define DS_NAM_FMT    "%19[a-zA-Z0-9_-]"
#define DS_NAM_SIZE   20

#define DST_FMT    "%19[A-Z]"
#define DST_SIZE   20

typedef struct ds_def_t {
    char      ds_nam[DS_NAM_SIZE];  /* Name of the data source (null terminated) */
    char      dst[DST_SIZE];    /* Type of data source (null terminated) */
    unival    par[10];  /* index of this array see ds_param_en */
} ds_def_t;

/****************************************************************************
 * POS 3: rra_def_t ( *  rra_cnt)         one for each store to be maintained
 ****************************************************************************/
enum cf_en { CF_AVERAGE = 0,    /* data consolidation functions */
    CF_MINIMUM,
    CF_MAXIMUM,
    CF_LAST,
    CF_HWPREDICT,
    /* An array of predictions using the seasonal 
     * Holt-Winters algorithm. Requires an RRA of type
     * CF_SEASONAL for this data source. */
    CF_SEASONAL,
    /* An array of seasonal effects. Requires an RRA of
     * type CF_HWPREDICT for this data source. */
    CF_DEVPREDICT,
    /* An array of deviation predictions based upon
     * smoothed seasonal deviations. Requires an RRA of
     * type CF_DEVSEASONAL for this data source. */
    CF_DEVSEASONAL,
    /* An array of smoothed seasonal deviations. Requires
     * an RRA of type CF_HWPREDICT for this data source.
     * */
    CF_FAILURES,
    /* HWPREDICT that follows a moving baseline */
    CF_MHWPREDICT
        /* new entries must come last !!! */
};

                       /* A binary array of failure indicators: 1 indicates
                        * that the number of violations in the prescribed
                        * window exceeded the prescribed threshold. */

#define MAX_RRA_PAR_EN 10
enum rra_par_en { RRA_cdp_xff_val = 0,  /* what part of the consolidated
                                         * datapoint must be known, to produce a
                                         * valid entry in the rra */
    /* CF_HWPREDICT: */
    RRA_hw_alpha = 1,
    /* exponential smoothing parameter for the intercept in
     * the Holt-Winters prediction algorithm. */
    RRA_hw_beta = 2,
    /* exponential smoothing parameter for the slope in
     * the Holt-Winters prediction algorithm. */

    RRA_dependent_rra_idx = 3,
    /* For CF_HWPREDICT: index of the RRA with the seasonal 
     * effects of the Holt-Winters algorithm (of type
     * CF_SEASONAL).
     * For CF_DEVPREDICT: index of the RRA with the seasonal
     * deviation predictions (of type CF_DEVSEASONAL).
     * For CF_SEASONAL: index of the RRA with the Holt-Winters
     * intercept and slope coefficient (of type CF_HWPREDICT).
     * For CF_DEVSEASONAL: index of the RRA with the 
     * Holt-Winters prediction (of type CF_HWPREDICT).
     * For CF_FAILURES: index of the CF_DEVSEASONAL array.
     * */

    /* CF_SEASONAL and CF_DEVSEASONAL: */
    RRA_seasonal_gamma = 1,
    /* exponential smoothing parameter for seasonal effects. */

    RRA_seasonal_smoothing_window = 2,
    /* fraction of the season to include in the running average
     * smoother */

    /* RRA_dependent_rra_idx = 3, */

    RRA_seasonal_smooth_idx = 4,
    /* an integer between 0 and row_count - 1 which
     * is index in the seasonal cycle for applying
     * the period smoother. */

    /* CF_FAILURES: */
    RRA_delta_pos = 1,  /* confidence bound scaling parameters */
    RRA_delta_neg = 2,
    /* RRA_dependent_rra_idx = 3, */
    RRA_window_len = 4,
    RRA_failure_threshold = 5
    /* For CF_FAILURES, number of violations within the last
     * window required to mark a failure. */
};

                    /* For CF_FAILURES, the length of the window for measuring
                     * failures. */

#define CF_NAM_FMT    "%19[A-Z]"
#define CF_NAM_SIZE   20

typedef struct rra_def_t {
    char      cf_nam[CF_NAM_SIZE];  /* consolidation function (null term) */
    unsigned long row_cnt;  /* number of entries in the store */
    unsigned long pdp_cnt;  /* how many primary data points are
                             * required for a consolidated data
                             * point?*/
    unival    par[MAX_RRA_PAR_EN];  /* index see rra_param_en */

} rra_def_t;


/****************************************************************************
 ****************************************************************************
 ****************************************************************************
 * LIVE PART OF THE HEADER. THIS WILL BE WRITTEN ON EVERY UPDATE         *
 ****************************************************************************
 ****************************************************************************
 ****************************************************************************/
/****************************************************************************
 * POS 4: live_head_t                    
 ****************************************************************************/

typedef struct live_head_t {
    time_t    last_up;  /* when was rrd last updated */
    long      last_up_usec; /* micro seconds part of the
                               update timestamp. Always >= 0 */
} live_head_t;


/****************************************************************************
 * POS 5: pdp_prep_t  (* ds_cnt)                     here we prepare the pdps 
 ****************************************************************************/
#define LAST_DS_LEN 30  /* DO NOT CHANGE THIS ... */

enum pdp_par_en { PDP_unkn_sec_cnt = 0, /* how many seconds of the current
                                         * pdp value is unknown data? */

    PDP_val
};                      /* current value of the pdp.
                           this depends on dst */

typedef struct pdp_prep_t {
    char      last_ds[LAST_DS_LEN]; /* the last reading from the data
                                     * source.  this is stored in ASCII
                                     * to cater for very large counters
                                     * we might encounter in connection
                                     * with SNMP. */
    unival    scratch[10];  /* contents according to pdp_par_en */
} pdp_prep_t;

/* data is passed from pdp to cdp when seconds since epoch modulo pdp_step == 0
   obviously the updates do not occur at these times only. Especially does the
   format allow for updates to occur at different times for each data source.
   The rules which makes this work is as follows:

   * DS updates may only occur at ever increasing points in time
   * When any DS update arrives after a cdp update time, the *previous*
     update cycle gets executed. All pdps are transfered to cdps and the
     cdps feed the rras where necessary. Only then the new DS value
     is loaded into the PDP.                                                   */


/****************************************************************************
 * POS 6: cdp_prep_t (* rra_cnt * ds_cnt )      data prep area for cdp values
 ****************************************************************************/
#define MAX_CDP_PAR_EN 10
#define MAX_CDP_FAILURES_IDX 8
/* max CDP scratch entries avail to record violations for a FAILURES RRA */
#define MAX_FAILURES_WINDOW_LEN 28
enum cdp_par_en { CDP_val = 0,
    /* the base_interval is always an
     * average */
    CDP_unkn_pdp_cnt,
    /* how many unknown pdp were
     * integrated. This and the cdp_xff
     * will decide if this is going to
     * be a UNKNOWN or a valid value */
    CDP_hw_intercept,
    /* Current intercept coefficient for the Holt-Winters
     * prediction algorithm. */
    CDP_hw_last_intercept,
    /* Last iteration intercept coefficient for the Holt-Winters
     * prediction algorihtm. */
    CDP_hw_slope,
    /* Current slope coefficient for the Holt-Winters
     * prediction algorithm. */
    CDP_hw_last_slope,
    /* Last iteration slope coeffient. */
    CDP_null_count,
    /* Number of sequential Unknown (DNAN) values + 1 preceding
     * the current prediction.
     * */
    CDP_last_null_count,
    /* Last iteration count of Unknown (DNAN) values. */
    CDP_primary_val = 8,
    /* optimization for bulk updates: the value of the first CDP
     * value to be written in the bulk update. */
    CDP_secondary_val = 9,
    /* optimization for bulk updates: the value of subsequent
     * CDP values to be written in the bulk update. */
    CDP_hw_seasonal = CDP_hw_intercept,
    /* Current seasonal coefficient for the Holt-Winters
     * prediction algorithm. This is stored in CDP prep to avoid
     * redundant seek operations. */
    CDP_hw_last_seasonal = CDP_hw_last_intercept,
    /* Last iteration seasonal coeffient. */
    CDP_seasonal_deviation = CDP_hw_intercept,
    CDP_last_seasonal_deviation = CDP_hw_last_intercept,
    CDP_init_seasonal = CDP_null_count
};

                   /* init_seasonal is a flag which when > 0, forces smoothing updates
                    * to occur when rra_ptr.cur_row == 0 */

typedef struct cdp_prep_t {
    unival    scratch[MAX_CDP_PAR_EN];
    /* contents according to cdp_par_en *
     * init state should be NAN */

} cdp_prep_t;

/****************************************************************************
 * POS 7: rra_ptr_t (* rra_cnt)       pointers to the current row in each rra
 ****************************************************************************/

typedef struct rra_ptr_t {
    unsigned long cur_row;  /* current row in the rra */
} rra_ptr_t;


/****************************************************************************
 ****************************************************************************
 * One single struct to hold all the others. For convenience.
 ****************************************************************************
 ****************************************************************************/
typedef struct rrd_t {
    stat_head_t *stat_head; /* the static header */
    ds_def_t *ds_def;   /* list of data source definitions */
    rra_def_t *rra_def; /* list of round robin archive def */
    live_head_t *live_head; /* rrd v >= 3 last_up with us */
    time_t   *legacy_last_up;   /* rrd v < 3 last_up time */
    pdp_prep_t *pdp_prep;   /* pdp data prep area */
    cdp_prep_t *cdp_prep;   /* cdp prep area */
    rra_ptr_t *rra_ptr; /* list of rra pointers */
    rrd_value_t *rrd_value; /* list of rrd values */
} rrd_t;

/****************************************************************************
 ****************************************************************************
 * AFTER the header section we have the DATA STORAGE AREA it is made up from
 * Consolidated Data Points organized in Round Robin Archives.
 ****************************************************************************
 ****************************************************************************

 *RRA 0
 (0,0) .................... ( ds_cnt -1 , 0)
 .
 . 
 .
 (0, row_cnt -1) ... (ds_cnt -1, row_cnt -1)

 *RRA 1
 *RRA 2

 *RRA rra_cnt -1
 
 ****************************************************************************/


#endif
