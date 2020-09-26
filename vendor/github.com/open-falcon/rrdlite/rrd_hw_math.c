/*****************************************************************************
 * rrd_hw_math.c  Math functions for Holt-Winters computations
 *****************************************************************************/

#include "rrd_tool.h"
#include "rrd_hw_math.h"

/*****************************************************************************
 * RRDtool supports both the additive and multiplicative Holt-Winters methods. 
 * The additive method makes predictions by adding seasonality to the baseline, 
 * whereas the multiplicative method multiplies the seasonality coefficient by 
 * the baseline to make a prediction. This file contains all the differences
 * between the additive and multiplicative methods, as well as a few math 
 * functions common to them both.
 ****************************************************************************/

/*****************************************************************************
 * Functions for additive Holt-Winters
 *****************************************************************************/

rrd_value_t hw_additive_calculate_prediction(
    rrd_value_t intercept,
    rrd_value_t slope,
    int null_count,
    rrd_value_t seasonal_coef)
{
    return intercept + slope * null_count + seasonal_coef;
}

rrd_value_t hw_additive_calculate_intercept(
    rrd_value_t hw_alpha,
    rrd_value_t observed,
    rrd_value_t seasonal_coef,
    unival *coefs)
{
    return hw_alpha * (observed - seasonal_coef)
        + (1 - hw_alpha) * (coefs[CDP_hw_intercept].u_val
                            +
                            (coefs[CDP_hw_slope].u_val) *
                            (coefs[CDP_null_count].u_cnt));
}

rrd_value_t hw_additive_calculate_seasonality(
    rrd_value_t hw_gamma,
    rrd_value_t observed,
    rrd_value_t intercept,
    rrd_value_t seasonal_coef)
{
    return hw_gamma * (observed - intercept)
        + (1 - hw_gamma) * seasonal_coef;
}

rrd_value_t hw_additive_init_seasonality(
    rrd_value_t seasonal_coef,
    rrd_value_t intercept)
{
    return seasonal_coef - intercept;
}

/*****************************************************************************
 * Functions for multiplicative Holt-Winters
 *****************************************************************************/

rrd_value_t hw_multiplicative_calculate_prediction(
    rrd_value_t intercept,
    rrd_value_t slope,
    int null_count,
    rrd_value_t seasonal_coef)
{
    return (intercept + slope * null_count) * seasonal_coef;
}

rrd_value_t hw_multiplicative_calculate_intercept(
    rrd_value_t hw_alpha,
    rrd_value_t observed,
    rrd_value_t seasonal_coef,
    unival *coefs)
{
    if (seasonal_coef <= 0) {
        return DNAN;
    }

    return hw_alpha * (observed / seasonal_coef)
        + (1 - hw_alpha) * (coefs[CDP_hw_intercept].u_val
                            +
                            (coefs[CDP_hw_slope].u_val) *
                            (coefs[CDP_null_count].u_cnt));
}

rrd_value_t hw_multiplicative_calculate_seasonality(
    rrd_value_t hw_gamma,
    rrd_value_t observed,
    rrd_value_t intercept,
    rrd_value_t seasonal_coef)
{
    if (intercept <= 0) {
        return DNAN;
    }

    return hw_gamma * (observed / intercept)
        + (1 - hw_gamma) * seasonal_coef;
}

rrd_value_t hw_multiplicative_init_seasonality(
    rrd_value_t seasonal_coef,
    rrd_value_t intercept)
{
    if (intercept <= 0) {
        return DNAN;
    }

    return seasonal_coef / intercept;
}

/*****************************************************************************
 * Math functions common to additive and multiplicative Holt-Winters
 *****************************************************************************/

rrd_value_t hw_calculate_slope(
    rrd_value_t hw_beta,
    unival *coefs)
{
    return hw_beta * (coefs[CDP_hw_intercept].u_val -
                      coefs[CDP_hw_last_intercept].u_val)
        + (1 - hw_beta) * coefs[CDP_hw_slope].u_val;
}

rrd_value_t hw_calculate_seasonal_deviation(
    rrd_value_t hw_gamma,
    rrd_value_t prediction,
    rrd_value_t observed,
    rrd_value_t last)
{
    return hw_gamma * fabs(prediction - observed)
        + (1 - hw_gamma) * last;
}

rrd_value_t hw_init_seasonal_deviation(
    rrd_value_t prediction,
    rrd_value_t observed)
{
    return fabs(prediction - observed);
}
