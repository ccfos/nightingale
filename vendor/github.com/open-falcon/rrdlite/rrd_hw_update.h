/*****************************************************************************
 * rrd_hw_update.h  Functions for updating a Holt-Winters RRA
 ****************************************************************************/

int       update_hwpredict(
    rrd_t *rrd,
    unsigned long cdp_idx,
    unsigned long rra_idx,
    unsigned long ds_idx,
    unsigned short CDP_scratch_idx,
    hw_functions_t * functions);

int       update_seasonal(
    rrd_t *rrd,
    unsigned long cdp_idx,
    unsigned long rra_idx,
    unsigned long ds_idx,
    unsigned short CDP_scratch_idx,
    rrd_value_t *seasonal_coef,
    hw_functions_t * functions);

int       update_devpredict(
    rrd_t *rrd,
    unsigned long cdp_idx,
    unsigned long rra_idx,
    unsigned long ds_idx,
    unsigned short CDP_scratch_idx);

int       update_devseasonal(
    rrd_t *rrd,
    unsigned long cdp_idx,
    unsigned long rra_idx,
    unsigned long ds_idx,
    unsigned short CDP_scratch_idx,
    rrd_value_t *seasonal_dev,
    hw_functions_t * functions);

int       update_failures(
    rrd_t *rrd,
    unsigned long cdp_idx,
    unsigned long rra_idx,
    unsigned long ds_idx,
    unsigned short CDP_scratch_idx,
    hw_functions_t * functions);
