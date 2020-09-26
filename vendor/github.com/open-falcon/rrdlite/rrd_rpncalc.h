/****************************************************************************
 * RRDtool 1.4.9  Copyright by Tobi Oetiker, 1997-2014
 ****************************************************************************
 * rrd_rpncalc.h  RPN calculator functions
 ****************************************************************************/
#ifndef _RRD_RPNCALC_H
#define _RRD_RPNCALC_H

/* WARNING: if new operators are added, they MUST be added at the very end of the list.
 * This is because COMPUTE (CDEF) DS store OP nodes by number (name is not
 * an option due to limited par array size). OP nodes must have the same
 * numeric values, otherwise the stored numbers will mean something different. */
enum op_en { OP_NUMBER = 0, OP_VARIABLE, OP_INF, OP_PREV, OP_NEGINF,
    OP_UNKN, OP_NOW, OP_TIME, OP_ADD, OP_MOD, OP_SUB, OP_MUL,
    OP_DIV, OP_SIN, OP_DUP, OP_EXC, OP_POP,
    OP_COS, OP_LOG, OP_EXP, OP_LT, OP_LE, OP_GT, OP_GE, OP_EQ, OP_IF,
    OP_MIN, OP_MAX, OP_LIMIT, OP_FLOOR, OP_CEIL,
    OP_UN, OP_END, OP_LTIME, OP_NE, OP_ISINF, OP_PREV_OTHER, OP_COUNT,
    OP_ATAN, OP_SQRT, OP_SORT, OP_REV, OP_TREND, OP_TRENDNAN,
    OP_ATAN2, OP_RAD2DEG, OP_DEG2RAD,
    OP_PREDICT,OP_PREDICTSIGMA,
    OP_AVG, OP_ABS, OP_ADDNAN,
    OP_MINNAN, OP_MAXNAN
};

typedef struct rpnp_t {
    enum op_en op;
    double    val;      /* value for a OP_NUMBER */
    long      ptr;      /* pointer into the gdes array for OP_VAR */
    double   *data;     /* pointer to the current value from OP_VAR DAS */
    long      ds_cnt;   /* data source count for data pointer */
    long      step;     /* time step for OP_VAR das */
} rpnp_t;

/* a compact representation of rpnp_t for computed data sources */
typedef struct rpn_cdefds_t {
    char      op;       /* rpn operator type */
    short     val;      /* used by OP_NUMBER and OP_VARIABLE */
} rpn_cdefds_t;

#define MAX_VNAME_LEN 255
#define DEF_NAM_FMT "%255[-_A-Za-z0-9]"

/* limit imposed by sizeof(rpn_cdefs_t) and rrd.ds_def.par */
#define DS_CDEF_MAX_RPN_NODES (int)(sizeof(unival)*10 / sizeof(rpn_cdefds_t))

typedef struct rpnstack_t {
    double   *s;
    long      dc_stacksize;
    long      dc_stackblock;
} rpnstack_t;

void      rpnstack_init(
    rpnstack_t *rpnstack);
void      rpnstack_free(
    rpnstack_t *rpnstack);

int      parseCDEF_DS(
    const char *def,
    rrd_t *rrd,
    int ds_idx);
long      lookup_DS(
    void *rrd_vptr,
    char *ds_name);

short     rpn_compact(
    rpnp_t *rpnp,
    rpn_cdefds_t **rpnc,
    short *count);
rpnp_t   *rpn_expand(
    rpn_cdefds_t *rpnc);
void      rpn_compact2str(
    rpn_cdefds_t *rpnc,
    ds_def_t *ds_def,
    char **str);
rpnp_t   *rpn_parse(
    void *key_hash,
    const char *const expr,
    long      (*lookup) (void *,
                         char *));
short     rpn_calc(
    rpnp_t *rpnp,
    rpnstack_t *rpnstack,
    long data_idx,
    rrd_value_t *output,
    int output_idx);

#endif
