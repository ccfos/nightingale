/*****************************************************************************
 * RRDtool 1.4.9  Copyright by Tobi Oetiker, 1997-2014
 *****************************************************************************
 * rrd_hw.h : Support for Holt-Winters Smoothing/ Aberrant Behavior Detection
 *****************************************************************************/

/* functions implemented in rrd_hw.c */
int       update_aberrant_CF(
    rrd_t *rrd,
    rrd_value_t pdp_val,
    enum cf_en current_cf,
    unsigned long cdp_idx,
    unsigned long rra_idx,
    unsigned long ds_idx,
    unsigned short CDP_scratch_idx,
    rrd_value_t *seasonal_coef);
int       create_hw_contingent_rras(
    rrd_t *rrd,
    unsigned short period,
    unsigned long hashed_name);
int       lookup_seasonal(
    rrd_t *rrd,
    unsigned long rra_idx,
    unsigned long rra_start,
    rrd_file_t *rrd_file,
    unsigned long offset,
    rrd_value_t **seasonal_coef);
int      erase_violations(
    rrd_t *rrd,
    unsigned long cdp_idx,
    unsigned long rra_idx);
int       apply_smoother(
    rrd_t *rrd,
    unsigned long rra_idx,
    unsigned long rra_start,
    rrd_file_t *rrd_file);
void      init_hwpredict_cdp(
    cdp_prep_t *);
void      init_seasonal_cdp(
    cdp_prep_t *);

#define BURNIN_CYCLES 3

/* a standard fixed-capacity FIFO queue implementation */
typedef struct FIFOqueue {
    rrd_value_t *queue;
    int       capacity, head, tail;
} FIFOqueue;

int       queue_alloc(
    FIFOqueue **q,
    int capacity);
void      queue_dealloc(
    FIFOqueue *q);
void      queue_push(
    FIFOqueue *q,
    rrd_value_t value);
int       queue_isempty(
    FIFOqueue *q);
rrd_value_t queue_pop(
    FIFOqueue *q);
