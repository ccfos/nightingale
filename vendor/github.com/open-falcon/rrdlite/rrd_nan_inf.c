int       done_nan = 0;
int       done_inf = 0;

double    dnan;
double    dinf;

#if defined(_WIN32) && !defined(__CYGWIN__) && !defined(__CYGWIN32__)
#include <math.h>
#include "rrd.h"

#define NAN_FUNC (double)fmod(0.0,0.0)
#define INF_FUNC (double)fabs((double)log(0.0))

#else
#include "rrd.h"

#define NAN_FUNC (double)(0.0/0.0)
#define INF_FUNC (double)(1.0/0.0)

#endif

double rrd_set_to_DNAN(
    void)
{
    if (!done_nan) {
        dnan = NAN_FUNC;
        done_nan = 1;
    }
    return dnan;
}

double rrd_set_to_DINF(
    void)
{
    if (!done_inf) {
        dinf = INF_FUNC;
        done_inf = 1;
    }
    return dinf;
}
