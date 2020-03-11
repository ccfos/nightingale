/*****************************************************************************
 * rrd_hw_update.c  Functions for updating a Holt-Winters RRA
 ****************************************************************************/

#include "rrd_tool.h"
#include "rrd_format.h"
#include "rrd_hw_math.h"
#include "rrd_hw_update.h"

static void init_slope_intercept(
    unival *coefs,
    unsigned short CDP_scratch_idx)
{
#ifdef DEBUG
    fprintf(stderr, "Initialization of slope/intercept\n");
#endif
    coefs[CDP_hw_intercept].u_val = coefs[CDP_scratch_idx].u_val;
    coefs[CDP_hw_last_intercept].u_val = coefs[CDP_scratch_idx].u_val;
    /* initialize the slope to 0 */
    coefs[CDP_hw_slope].u_val = 0.0;
    coefs[CDP_hw_last_slope].u_val = 0.0;
    /* initialize null count to 1 */
    coefs[CDP_null_count].u_cnt = 1;
    coefs[CDP_last_null_count].u_cnt = 1;
}

static int hw_is_violation(
    rrd_value_t observed,
    rrd_value_t prediction,
    rrd_value_t deviation,
    rrd_value_t delta_pos,
    rrd_value_t delta_neg)
{
    return (observed > prediction + delta_pos * deviation
            || observed < prediction - delta_neg * deviation);
}

int update_hwpredict(
    rrd_t *rrd,
    unsigned long cdp_idx,
    unsigned long rra_idx,
    unsigned long ds_idx,
    unsigned short CDP_scratch_idx,
    hw_functions_t * functions)
{
    rrd_value_t prediction;
    unsigned long dependent_rra_idx, seasonal_cdp_idx;
    unival   *coefs = rrd->cdp_prep[cdp_idx].scratch;
    rra_def_t *current_rra = &(rrd->rra_def[rra_idx]);
    rrd_value_t seasonal_coef;

    /* save coefficients from current prediction */
    coefs[CDP_hw_last_intercept].u_val = coefs[CDP_hw_intercept].u_val;
    coefs[CDP_hw_last_slope].u_val = coefs[CDP_hw_slope].u_val;
    coefs[CDP_last_null_count].u_cnt = coefs[CDP_null_count].u_cnt;

    /* retrieve the current seasonal coef */
    dependent_rra_idx = current_rra->par[RRA_dependent_rra_idx].u_cnt;
    seasonal_cdp_idx = dependent_rra_idx * (rrd->stat_head->ds_cnt) + ds_idx;

    seasonal_coef = (dependent_rra_idx < rra_idx)
        ? rrd->cdp_prep[seasonal_cdp_idx].scratch[CDP_hw_last_seasonal].u_val
        : rrd->cdp_prep[seasonal_cdp_idx].scratch[CDP_hw_seasonal].u_val;

    /* compute the prediction */
    if (isnan(coefs[CDP_hw_intercept].u_val)
        || isnan(coefs[CDP_hw_slope].u_val)
        || isnan(seasonal_coef)) {
        prediction = DNAN;

        /* bootstrap initialization of slope and intercept */
        if (isnan(coefs[CDP_hw_intercept].u_val) &&
            !isnan(coefs[CDP_scratch_idx].u_val)) {
            init_slope_intercept(coefs, CDP_scratch_idx);
        }
        /* if seasonal coefficient is NA, then don't update intercept, slope */
    } else {
        prediction = functions->predict(coefs[CDP_hw_intercept].u_val,
                                        coefs[CDP_hw_slope].u_val,
                                        coefs[CDP_null_count].u_cnt,
                                        seasonal_coef);
#ifdef DEBUG
        fprintf(stderr,
                "computed prediction: %f (intercept %f, slope %f, season %f)\n",
                prediction, coefs[CDP_hw_intercept].u_val,
                coefs[CDP_hw_slope].u_val, seasonal_coef);
#endif
        if (isnan(coefs[CDP_scratch_idx].u_val)) {
            /* NA value, no updates of intercept, slope;
             * increment the null count */
            (coefs[CDP_null_count].u_cnt)++;
        } else {
            /* update the intercept */
            coefs[CDP_hw_intercept].u_val =
                functions->intercept(current_rra->par[RRA_hw_alpha].u_val,
                                     coefs[CDP_scratch_idx].u_val,
                                     seasonal_coef, coefs);

            /* update the slope */
            coefs[CDP_hw_slope].u_val =
                functions->slope(current_rra->par[RRA_hw_beta].u_val, coefs);

            /* reset the null count */
            coefs[CDP_null_count].u_cnt = 1;
#ifdef DEBUG
            fprintf(stderr, "Updating intercept = %f, slope = %f\n",
                    coefs[CDP_hw_intercept].u_val, coefs[CDP_hw_slope].u_val);
#endif
        }
    }

    /* store the prediction for writing */
    coefs[CDP_scratch_idx].u_val = prediction;
    return 0;
}

int update_seasonal(
    rrd_t *rrd,
    unsigned long cdp_idx,
    unsigned long rra_idx,
    unsigned long ds_idx,
    unsigned short CDP_scratch_idx,
    rrd_value_t *seasonal_coef,
    hw_functions_t * functions)
{
/* TODO: extract common if subblocks in the wake of I/O optimization */
    rrd_value_t intercept, seasonal;
    rra_def_t *current_rra = &(rrd->rra_def[rra_idx]);
    rra_def_t *hw_rra =
        &(rrd->rra_def[current_rra->par[RRA_dependent_rra_idx].u_cnt]);

    /* obtain cdp_prep index for HWPREDICT */
    unsigned long hw_cdp_idx = (current_rra->par[RRA_dependent_rra_idx].u_cnt)
        * (rrd->stat_head->ds_cnt) + ds_idx;
    unival   *coefs = rrd->cdp_prep[hw_cdp_idx].scratch;

    /* update seasonal coefficient in cdp prep areas */
    seasonal = rrd->cdp_prep[cdp_idx].scratch[CDP_hw_seasonal].u_val;
    rrd->cdp_prep[cdp_idx].scratch[CDP_hw_last_seasonal].u_val = seasonal;
    rrd->cdp_prep[cdp_idx].scratch[CDP_hw_seasonal].u_val =
        seasonal_coef[ds_idx];

    if (isnan(rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val)) {
        /* no update, store the old value unchanged,
         * doesn't matter if it is NA */
        rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val = seasonal;
        return 0;
    }

    /* update seasonal value for disk */
    if (current_rra->par[RRA_dependent_rra_idx].u_cnt < rra_idx) {
        /* associated HWPREDICT has already been updated */
        /* check for possible NA values */
        if (isnan(coefs[CDP_hw_last_intercept].u_val)
            || isnan(coefs[CDP_hw_last_slope].u_val)) {
            /* this should never happen, as HWPREDICT was already updated */
            rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val = DNAN;
        } else if (isnan(seasonal)) {
            /* initialization: intercept is not currently being updated */
#ifdef DEBUG
            fprintf(stderr, "Initialization of seasonal coef %lu\n",
                    rrd->rra_ptr[rra_idx].cur_row);
#endif
            rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val =
                functions->init_seasonality(rrd->cdp_prep[cdp_idx].
                                            scratch[CDP_scratch_idx].u_val,
                                            coefs[CDP_hw_last_intercept].
                                            u_val);
        } else {
            intercept = coefs[CDP_hw_intercept].u_val;

            rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val =
                functions->seasonality(current_rra->par[RRA_seasonal_gamma].
                                       u_val,
                                       rrd->cdp_prep[cdp_idx].
                                       scratch[CDP_scratch_idx].u_val,
                                       intercept, seasonal);
#ifdef DEBUG
            fprintf(stderr,
                    "Updating seasonal = %f (params: gamma %f, new intercept %f, old seasonal %f)\n",
                    rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val,
                    current_rra->par[RRA_seasonal_gamma].u_val,
                    intercept, seasonal);
#endif
        }
    } else {
        /* SEASONAL array is updated first, which means the new intercept
         * hasn't be computed; so we compute it here. */

        /* check for possible NA values */
        if (isnan(coefs[CDP_hw_intercept].u_val)
            || isnan(coefs[CDP_hw_slope].u_val)) {
            /* Initialization of slope and intercept will occur.
             * force seasonal coefficient to 0 or 1. */
            rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val =
                functions->identity;
        } else if (isnan(seasonal)) {
            /* initialization: intercept will not be updated
             * CDP_hw_intercept = CDP_hw_last_intercept; just need to 
             * subtract/divide by this baseline value. */
#ifdef DEBUG
            fprintf(stderr, "Initialization of seasonal coef %lu\n",
                    rrd->rra_ptr[rra_idx].cur_row);
#endif
            rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val =
                functions->init_seasonality(rrd->cdp_prep[cdp_idx].
                                            scratch[CDP_scratch_idx].u_val,
                                            coefs[CDP_hw_intercept].u_val);
        } else {
            /* Note that we must get CDP_scratch_idx from SEASONAL array, as CDP_scratch_idx
             * for HWPREDICT array will be DNAN. */
            intercept = functions->intercept(hw_rra->par[RRA_hw_alpha].u_val,
                                             rrd->cdp_prep[cdp_idx].
                                             scratch[CDP_scratch_idx].u_val,
                                             seasonal, coefs);

            rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val =
                functions->seasonality(current_rra->par[RRA_seasonal_gamma].
                                       u_val,
                                       rrd->cdp_prep[cdp_idx].
                                       scratch[CDP_scratch_idx].u_val,
                                       intercept, seasonal);
        }
    }
#ifdef DEBUG
    fprintf(stderr, "seasonal coefficient set= %f\n",
            rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val);
#endif
    return 0;
}

int update_devpredict(
    rrd_t *rrd,
    unsigned long cdp_idx,
    unsigned long rra_idx,
    unsigned long ds_idx,
    unsigned short CDP_scratch_idx)
{
    /* there really isn't any "update" here; the only reason this information
     * is stored separately from DEVSEASONAL is to preserve deviation predictions
     * for a longer duration than one seasonal cycle. */
    unsigned long seasonal_cdp_idx =
        (rrd->rra_def[rra_idx].par[RRA_dependent_rra_idx].u_cnt)
        * (rrd->stat_head->ds_cnt) + ds_idx;

    if (rrd->rra_def[rra_idx].par[RRA_dependent_rra_idx].u_cnt < rra_idx) {
        /* associated DEVSEASONAL array already updated */
        rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val
            =
            rrd->cdp_prep[seasonal_cdp_idx].
            scratch[CDP_last_seasonal_deviation].u_val;
    } else {
        /* associated DEVSEASONAL not yet updated */
        rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val
            =
            rrd->cdp_prep[seasonal_cdp_idx].scratch[CDP_seasonal_deviation].
            u_val;
    }
    return 0;
}

int update_devseasonal(
    rrd_t *rrd,
    unsigned long cdp_idx,
    unsigned long rra_idx,
    unsigned long ds_idx,
    unsigned short CDP_scratch_idx,
    rrd_value_t *seasonal_dev,
    hw_functions_t * functions)
{
    rrd_value_t prediction = 0, seasonal_coef = DNAN;
    rra_def_t *current_rra = &(rrd->rra_def[rra_idx]);

    /* obtain cdp_prep index for HWPREDICT */
    unsigned long hw_rra_idx = current_rra->par[RRA_dependent_rra_idx].u_cnt;
    unsigned long hw_cdp_idx = hw_rra_idx * (rrd->stat_head->ds_cnt) + ds_idx;
    unsigned long seasonal_cdp_idx;
    unival   *coefs = rrd->cdp_prep[hw_cdp_idx].scratch;

    rrd->cdp_prep[cdp_idx].scratch[CDP_last_seasonal_deviation].u_val =
        rrd->cdp_prep[cdp_idx].scratch[CDP_seasonal_deviation].u_val;
    /* retrieve the next seasonal deviation value, could be NA */
    rrd->cdp_prep[cdp_idx].scratch[CDP_seasonal_deviation].u_val =
        seasonal_dev[ds_idx];

    /* retrieve the current seasonal_coef (not to be confused with the
     * current seasonal deviation). Could make this more readable by introducing
     * some wrapper functions. */
    seasonal_cdp_idx =
        (rrd->rra_def[hw_rra_idx].par[RRA_dependent_rra_idx].u_cnt)
        * (rrd->stat_head->ds_cnt) + ds_idx;
    if (rrd->rra_def[hw_rra_idx].par[RRA_dependent_rra_idx].u_cnt < rra_idx)
        /* SEASONAL array already updated */
        seasonal_coef =
            rrd->cdp_prep[seasonal_cdp_idx].scratch[CDP_hw_last_seasonal].
            u_val;
    else
        /* SEASONAL array not yet updated */
        seasonal_coef =
            rrd->cdp_prep[seasonal_cdp_idx].scratch[CDP_hw_seasonal].u_val;

    /* compute the abs value of the difference between the prediction and
     * observed value */
    if (hw_rra_idx < rra_idx) {
        /* associated HWPREDICT has already been updated */
        if (isnan(coefs[CDP_hw_last_intercept].u_val) ||
            isnan(coefs[CDP_hw_last_slope].u_val) || isnan(seasonal_coef)) {
            /* one of the prediction values is uinitialized */
            rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val = DNAN;
            return 0;
        } else {
            prediction =
                functions->predict(coefs[CDP_hw_last_intercept].u_val,
                                   coefs[CDP_hw_last_slope].u_val,
                                   coefs[CDP_last_null_count].u_cnt,
                                   seasonal_coef);
        }
    } else {
        /* associated HWPREDICT has NOT been updated */
        if (isnan(coefs[CDP_hw_intercept].u_val) ||
            isnan(coefs[CDP_hw_slope].u_val) || isnan(seasonal_coef)) {
            /* one of the prediction values is uinitialized */
            rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val = DNAN;
            return 0;
        } else {
            prediction = functions->predict(coefs[CDP_hw_intercept].u_val,
                                            coefs[CDP_hw_slope].u_val,
                                            coefs[CDP_null_count].u_cnt,
                                            seasonal_coef);
        }
    }

    if (isnan(rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val)) {
        /* no update, store existing value unchanged, doesn't
         * matter if it is NA */
        rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val =
            rrd->cdp_prep[cdp_idx].scratch[CDP_last_seasonal_deviation].u_val;
    } else
        if (isnan
            (rrd->cdp_prep[cdp_idx].scratch[CDP_last_seasonal_deviation].
             u_val)) {
        /* initialization */
#ifdef DEBUG
        fprintf(stderr, "Initialization of seasonal deviation\n");
#endif
        rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val =
            functions->init_seasonal_deviation(prediction,
                                               rrd->cdp_prep[cdp_idx].
                                               scratch[CDP_scratch_idx].
                                               u_val);
    } else {
        /* exponential smoothing update */
        rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val =
            functions->seasonal_deviation(rrd->rra_def[rra_idx].
                                          par[RRA_seasonal_gamma].u_val,
                                          prediction,
                                          rrd->cdp_prep[cdp_idx].
                                          scratch[CDP_scratch_idx].u_val,
                                          rrd->cdp_prep[cdp_idx].
                                          scratch
                                          [CDP_last_seasonal_deviation].
                                          u_val);
    }
    return 0;
}

/* Check for a failure based on a threshold # of violations within the specified
 * window. */
int update_failures(
    rrd_t *rrd,
    unsigned long cdp_idx,
    unsigned long rra_idx,
    unsigned long ds_idx,
    unsigned short CDP_scratch_idx,
    hw_functions_t * functions)
{
    /* detection of a violation depends on 3 RRAs:
     * HWPREDICT, SEASONAL, and DEVSEASONAL */
    rra_def_t *current_rra = &(rrd->rra_def[rra_idx]);
    unsigned long dev_rra_idx = current_rra->par[RRA_dependent_rra_idx].u_cnt;
    rra_def_t *dev_rra = &(rrd->rra_def[dev_rra_idx]);
    unsigned long hw_rra_idx = dev_rra->par[RRA_dependent_rra_idx].u_cnt;
    rra_def_t *hw_rra = &(rrd->rra_def[hw_rra_idx]);
    unsigned long seasonal_rra_idx = hw_rra->par[RRA_dependent_rra_idx].u_cnt;
    unsigned long temp_cdp_idx;
    rrd_value_t deviation = DNAN;
    rrd_value_t seasonal_coef = DNAN;
    rrd_value_t prediction = DNAN;
    char      violation = 0;
    unsigned short violation_cnt = 0, i;
    char     *violations_array;

    /* usual checks to determine the order of the RRAs */
    temp_cdp_idx = dev_rra_idx * (rrd->stat_head->ds_cnt) + ds_idx;
    if (rra_idx < seasonal_rra_idx) {
        /* DEVSEASONAL not yet updated */
        deviation =
            rrd->cdp_prep[temp_cdp_idx].scratch[CDP_seasonal_deviation].u_val;
    } else {
        /* DEVSEASONAL already updated */
        deviation =
            rrd->cdp_prep[temp_cdp_idx].scratch[CDP_last_seasonal_deviation].
            u_val;
    }
    if (!isnan(deviation)) {

        temp_cdp_idx = seasonal_rra_idx * (rrd->stat_head->ds_cnt) + ds_idx;
        if (rra_idx < seasonal_rra_idx) {
            /* SEASONAL not yet updated */
            seasonal_coef =
                rrd->cdp_prep[temp_cdp_idx].scratch[CDP_hw_seasonal].u_val;
        } else {
            /* SEASONAL already updated */
            seasonal_coef =
                rrd->cdp_prep[temp_cdp_idx].scratch[CDP_hw_last_seasonal].
                u_val;
        }
        /* in this code block, we know seasonal coef is not DNAN, because deviation is not
         * null */

        temp_cdp_idx = hw_rra_idx * (rrd->stat_head->ds_cnt) + ds_idx;
        if (rra_idx < hw_rra_idx) {
            /* HWPREDICT not yet updated */
            prediction =
                functions->predict(rrd->cdp_prep[temp_cdp_idx].
                                   scratch[CDP_hw_intercept].u_val,
                                   rrd->cdp_prep[temp_cdp_idx].
                                   scratch[CDP_hw_slope].u_val,
                                   rrd->cdp_prep[temp_cdp_idx].
                                   scratch[CDP_null_count].u_cnt,
                                   seasonal_coef);
        } else {
            /* HWPREDICT already updated */
            prediction =
                functions->predict(rrd->cdp_prep[temp_cdp_idx].
                                   scratch[CDP_hw_last_intercept].u_val,
                                   rrd->cdp_prep[temp_cdp_idx].
                                   scratch[CDP_hw_last_slope].u_val,
                                   rrd->cdp_prep[temp_cdp_idx].
                                   scratch[CDP_last_null_count].u_cnt,
                                   seasonal_coef);
        }

        /* determine if the observed value is a violation */
        if (!isnan(rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val)) {
            if (hw_is_violation
                (rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val,
                 prediction, deviation, current_rra->par[RRA_delta_pos].u_val,
                 current_rra->par[RRA_delta_neg].u_val)) {
                violation = 1;
            }
        } else {
            violation = 1;  /* count DNAN values as violations */
        }

    }

    /* determine if a failure has occurred and update the failure array */
    violation_cnt = violation;
    violations_array = (char *) ((void *) rrd->cdp_prep[cdp_idx].scratch);
    for (i = current_rra->par[RRA_window_len].u_cnt; i > 1; i--) {
        /* shift */
        violations_array[i - 1] = violations_array[i - 2];
        violation_cnt += violations_array[i - 1];
    }
    violations_array[0] = violation;

    if (violation_cnt < current_rra->par[RRA_failure_threshold].u_cnt)
        /* not a failure */
        rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val = 0.0;
    else
        rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val = 1.0;

    return (rrd->cdp_prep[cdp_idx].scratch[CDP_scratch_idx].u_val);
}
