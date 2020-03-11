/*****************************************************************************
 * rrd_hw_math.h  Math functions for Holt-Winters computations
 *****************************************************************************/

#include "rrd.h"
#include "rrd_format.h"

/* since /usr/include/bits/mathcalls.h:265 defines gamma already */
#define gamma hw_gamma

/*****************************************************************************
 * Functions for additive Holt-Winters
 *****************************************************************************/

rrd_value_t hw_additive_calculate_prediction(
    rrd_value_t intercept,
    rrd_value_t slope,
    int null_count,
    rrd_value_t seasonal_coef);

rrd_value_t hw_additive_calculate_intercept(
    rrd_value_t alpha,
    rrd_value_t scratch,
    rrd_value_t seasonal_coef,
    unival *coefs);

rrd_value_t hw_additive_calculate_seasonality(
    rrd_value_t gamma,
    rrd_value_t scratch,
    rrd_value_t intercept,
    rrd_value_t seasonal_coef);

rrd_value_t hw_additive_init_seasonality(
    rrd_value_t seasonal_coef,
    rrd_value_t intercept);

/*****************************************************************************
 * Functions for multiplicative Holt-Winters
 *****************************************************************************/

rrd_value_t hw_multiplicative_calculate_prediction(
    rrd_value_t intercept,
    rrd_value_t slope,
    int null_count,
    rrd_value_t seasonal_coef);

rrd_value_t hw_multiplicative_calculate_intercept(
    rrd_value_t alpha,
    rrd_value_t scratch,
    rrd_value_t seasonal_coef,
    unival *coefs);

rrd_value_t hw_multiplicative_calculate_seasonality(
    rrd_value_t gamma,
    rrd_value_t scratch,
    rrd_value_t intercept,
    rrd_value_t seasonal_coef);

rrd_value_t hw_multiplicative_init_seasonality(
    rrd_value_t seasonal_coef,
    rrd_value_t intercept);

/*****************************************************************************
 * Math functions common to additive and multiplicative Holt-Winters
 *****************************************************************************/

rrd_value_t hw_calculate_slope(
    rrd_value_t beta,
    unival *coefs);

rrd_value_t hw_calculate_seasonal_deviation(
    rrd_value_t gamma,
    rrd_value_t prediction,
    rrd_value_t observed,
    rrd_value_t last);

rrd_value_t hw_init_seasonal_deviation(
    rrd_value_t prediction,
    rrd_value_t observed);


/* Function container */

typedef struct hw_functions_t {
    rrd_value_t (
    *predict) (
    rrd_value_t intercept,
    rrd_value_t slope,
    int null_count,
    rrd_value_t seasonal_coef);

    rrd_value_t (
    *intercept) (
    rrd_value_t alpha,
    rrd_value_t observed,
    rrd_value_t seasonal_coef,
    unival *coefs);

    rrd_value_t (
    *slope)   (
    rrd_value_t beta,
    unival *coefs);

    rrd_value_t (
    *seasonality) (
    rrd_value_t gamma,
    rrd_value_t observed,
    rrd_value_t intercept,
    rrd_value_t seasonal_coef);

    rrd_value_t (
    *init_seasonality) (
    rrd_value_t seasonal_coef,
    rrd_value_t intercept);

    rrd_value_t (
    *seasonal_deviation) (
    rrd_value_t gamma,
    rrd_value_t prediction,
    rrd_value_t observed,
    rrd_value_t last);

    rrd_value_t (
    *init_seasonal_deviation) (
    rrd_value_t prediction,
    rrd_value_t observed);

    rrd_value_t identity;
} hw_functions_t;


#undef gamma
