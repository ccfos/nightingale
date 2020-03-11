/****************************************************************************
 * RRDtool 1.4.9  Copyright by Tobi Oetiker, 1997-2014
 ****************************************************************************
 * rrd_rpncalc.c  RPN calculator functions
 ****************************************************************************/

#include <limits.h>
#include <locale.h>
#include <stdlib.h>

#include "rrd_tool.h"
#include "rrd_rpncalc.h"
// #include "rrd_graph.h"

short     addop2str(
    enum op_en op,
    enum op_en op_type,
    char *op_str,
    char **result_str,
    unsigned short *offset);
int       tzoffset(
    time_t);            /* used to implement LTIME */

short rpn_compact(
    rpnp_t *rpnp,
    rpn_cdefds_t **rpnc,
    short *count)
{
    short     i;

    *count = 0;
    /* count the number of rpn nodes */
    while (rpnp[*count].op != OP_END)
        (*count)++;
    if (++(*count) > DS_CDEF_MAX_RPN_NODES) {
        return -RRD_ERR_DATA1;
    }

    /* allocate memory */
    *rpnc = (rpn_cdefds_t *) calloc(*count, sizeof(rpn_cdefds_t));
    for (i = 0; rpnp[i].op != OP_END; i++) {
        (*rpnc)[i].op = (char) rpnp[i].op;
        if (rpnp[i].op == OP_NUMBER) {
            /* rpnp.val is a double, rpnc.val is a short */
            double    temp = floor(rpnp[i].val);

			if (temp < SHRT_MIN || temp > SHRT_MAX || temp != rpnp[i].val) {
                free(*rpnc);
                return -RRD_ERR_DATA2;
            }
            (*rpnc)[i].val = (short) temp;
        } else if (rpnp[i].op == OP_VARIABLE || rpnp[i].op == OP_PREV_OTHER) {
            (*rpnc)[i].val = (short) rpnp[i].ptr;
        }
    }
    /* terminate the sequence */
    (*rpnc)[(*count) - 1].op = OP_END;
    return 0;
}

rpnp_t   *rpn_expand( rpn_cdefds_t *rpnc) {
    short     i;
    rpnp_t   *rpnp;

    /* DS_CDEF_MAX_RPN_NODES is small, so at the expense of some wasted
     * memory we avoid any reallocs */
    rpnp = (rpnp_t *) calloc(DS_CDEF_MAX_RPN_NODES, sizeof(rpnp_t));
    if (rpnp == NULL) {
		//RRD_ERR_MALLOC17
        return NULL;
    }
    for (i = 0; rpnc[i].op != OP_END; ++i) {
        rpnp[i].op = (enum op_en)rpnc[i].op;
        if (rpnp[i].op == OP_NUMBER) {
            rpnp[i].val = (double) rpnc[i].val;
        } else if (rpnp[i].op == OP_VARIABLE || rpnp[i].op == OP_PREV_OTHER) {
            rpnp[i].ptr = (long) rpnc[i].val;
        }
    }
    /* terminate the sequence */
    rpnp[i].op = OP_END;
    return rpnp;
}

/* rpn_compact2str: convert a compact sequence of RPN operator nodes back
 * into a CDEF string. This function is used by rrd_dump.
 * arguments:
 *  rpnc: an array of compact RPN operator nodes
 *  ds_def: a pointer to the data source definition section of an RRD header
 *   for lookup of data source names by index
 *  str: out string, memory is allocated by the function, must be freed by the
 *   the caller */
void rpn_compact2str( rpn_cdefds_t *rpnc, ds_def_t *ds_def, char **str) {
    unsigned short i, offset = 0;
    char      buffer[7];    /* short as a string */

    for (i = 0; rpnc[i].op != OP_END; i++) {
        if (i > 0)
            (*str)[offset++] = ',';

#define add_op(VV,VVV) \
	  if (addop2str((enum op_en)(rpnc[i].op), VV, VVV, str, &offset) == 1) continue;

        if (rpnc[i].op == OP_NUMBER) {
            /* convert a short into a string */
#if defined(_WIN32) && !defined(__CYGWIN__) && !defined(__CYGWIN32__)
            _itoa(rpnc[i].val, buffer, 10);
#else
            sprintf(buffer, "%d", rpnc[i].val);
#endif
            add_op(OP_NUMBER, buffer)
        }

        if (rpnc[i].op == OP_VARIABLE) {
            char     *ds_name = ds_def[rpnc[i].val].ds_nam;

            add_op(OP_VARIABLE, ds_name)
        }

        if (rpnc[i].op == OP_PREV_OTHER) {
            char     *ds_name = ds_def[rpnc[i].val].ds_nam;

            add_op(OP_VARIABLE, ds_name)
        }
#undef add_op

#define add_op(VV,VVV) \
	  if (addop2str((enum op_en)rpnc[i].op, VV, #VVV, str, &offset) == 1) continue;

        add_op(OP_ADD, +)
            add_op(OP_SUB, -)
            add_op(OP_MUL, *)
            add_op(OP_DIV, /)
            add_op(OP_MOD, %)
            add_op(OP_SIN, SIN)
            add_op(OP_COS, COS)
            add_op(OP_LOG, LOG)
            add_op(OP_FLOOR, FLOOR)
            add_op(OP_CEIL, CEIL)
            add_op(OP_EXP, EXP)
            add_op(OP_DUP, DUP)
            add_op(OP_EXC, EXC)
            add_op(OP_POP, POP)
            add_op(OP_LT, LT)
            add_op(OP_LE, LE)
            add_op(OP_GT, GT)
            add_op(OP_GE, GE)
            add_op(OP_EQ, EQ)
            add_op(OP_IF, IF)
            add_op(OP_MIN, MIN)
            add_op(OP_MAX, MAX)
            add_op(OP_LIMIT, LIMIT)
            add_op(OP_UNKN, UNKN)
            add_op(OP_UN, UN)
            add_op(OP_NEGINF, NEGINF)
            add_op(OP_NE, NE)
            add_op(OP_PREV, PREV)
            add_op(OP_INF, INF)
            add_op(OP_ISINF, ISINF)
            add_op(OP_NOW, NOW)
            add_op(OP_LTIME, LTIME)
            add_op(OP_TIME, TIME)
            add_op(OP_ATAN2, ATAN2)
            add_op(OP_ATAN, ATAN)
            add_op(OP_SQRT, SQRT)
            add_op(OP_SORT, SORT)
            add_op(OP_REV, REV)
            add_op(OP_TREND, TREND)
            add_op(OP_TRENDNAN, TRENDNAN)
            add_op(OP_PREDICT, PREDICT)
            add_op(OP_PREDICTSIGMA, PREDICTSIGMA)
            add_op(OP_RAD2DEG, RAD2DEG)
            add_op(OP_DEG2RAD, DEG2RAD)
            add_op(OP_AVG, AVG)
            add_op(OP_ABS, ABS)
            add_op(OP_ADDNAN, ADDNAN)
            add_op(OP_MINNAN, MINNAN)
            add_op(OP_MAXNAN, MAXNAN)
#undef add_op
    }
    (*str)[offset] = '\0';

}

short addop2str( enum op_en op, enum op_en op_type, char *op_str,
    char **result_str, unsigned short *offset) {
    if (op == op_type) {
        short     op_len;

        op_len = strlen(op_str);
        *result_str = (char *) rrd_realloc(*result_str,
                                           (op_len + 1 +
                                            *offset) * sizeof(char));
        if (*result_str == NULL) {
            return -RRD_ERR_MALLOC16;
        }
        strncpy(&((*result_str)[*offset]), op_str, op_len);
        *offset += op_len;
        return 1;
    }
    return 0;
}

int parseCDEF_DS( const char *def, rrd_t *rrd, int ds_idx) {
    rpnp_t   *rpnp = NULL;
    rpn_cdefds_t *rpnc = NULL;
    short     count, i;

    rpnp = rpn_parse((void *) rrd, def, &lookup_DS);
    if (rpnp == NULL) {
        return -RRD_ERR_PARSE1;
    }
    /* Check for OP nodes not permitted in COMPUTE DS.
     * Moved this check from within rpn_compact() because it really is
     * COMPUTE DS specific. This is less efficient, but creation doesn't
     * occur too often. */
    for (i = 0; rpnp[i].op != OP_END; i++) {
        if (rpnp[i].op == OP_TIME || rpnp[i].op == OP_LTIME ||
            rpnp[i].op == OP_PREV || rpnp[i].op == OP_COUNT ||
            rpnp[i].op == OP_TREND || rpnp[i].op == OP_TRENDNAN ||
            rpnp[i].op == OP_PREDICT || rpnp[i].op ==  OP_PREDICTSIGMA ) {
            free(rpnp);
            return -RRD_ERR_DS;
        }
    }
    if (rpn_compact(rpnp, &rpnc, &count) == -1) {
        free(rpnp);
        return 0;
    }
    /* copy the compact rpn representation over the ds_def par array */
    memcpy((void *) &(rrd->ds_def[ds_idx].par[DS_cdef]),
           (void *) rpnc, count * sizeof(rpn_cdefds_t));
    free(rpnp);
    free(rpnc);
	return 0;
}

/* lookup a data source name in the rrd struct and return the index,
 * should use ds_match() here except:
 * (1) need a void * pointer to the rrd
 * (2) error handling is left to the caller
 */
long lookup_DS(
    void *rrd_vptr,
    char *ds_name)
{
    unsigned int i;
    rrd_t    *rrd;

    rrd = (rrd_t *) rrd_vptr;

    for (i = 0; i < rrd->stat_head->ds_cnt; ++i) {
        if (strcmp(ds_name, rrd->ds_def[i].ds_nam) == 0)
            return i;
    }
    /* the caller handles a bad data source name in the rpn string */
    return -1;
}

/* rpn_parse : parse a string and generate a rpnp array; modified
 * str2rpn() originally included in rrd_graph.c
 * arguments:
 * key_hash: a transparent argument passed to lookup(); conceptually this
 *    is a hash object for lookup of a numeric key given a variable name
 * expr: the string RPN expression, including variable names
 * lookup(): a function that retrieves a numeric key given a variable name
 */
rpnp_t   *rpn_parse( void *key_hash, const char *const expr_const,
    long      (*lookup) (void *, char *)) {
    int       pos = 0;
    char     *expr;
    long      steps = -1;
    rpnp_t   *rpnp;
    char      vname[MAX_VNAME_LEN + 10];
    char     *old_locale;
	int r, ret = 0;

    old_locale = setlocale(LC_NUMERIC, "C");

    rpnp = NULL;
    expr = (char *) expr_const;

    while (*expr) {
        if ((rpnp = (rpnp_t *) rrd_realloc(rpnp, (++steps + 2) *
                                           sizeof(rpnp_t))) == NULL) {
            setlocale(LC_NUMERIC, old_locale);
            return NULL;
        }

        else if ((sscanf(expr, "%lf%n", &rpnp[steps].val, &pos) == 1)
                 && (expr[pos] == ',')) {
            rpnp[steps].op = OP_NUMBER;
            expr += pos;
        }
#define match_op(VV,VVV) \
        else if (strncmp(expr, #VVV, strlen(#VVV))==0 && ( expr[strlen(#VVV)] == ',' || expr[strlen(#VVV)] == '\0' )){ \
            rpnp[steps].op = VV; \
            expr+=strlen(#VVV); \
    	}

#define match_op_param(VV,VVV) \
        else if (sscanf(expr, #VVV "(" DEF_NAM_FMT ")",vname) == 1) { \
          int length = 0; \
          if ((length = strlen(#VVV)+strlen(vname)+2, \
              expr[length] == ',' || expr[length] == '\0') ) { \
             rpnp[steps].op = VV; \
             rpnp[steps].ptr = (*lookup)(key_hash,vname); \
             if (rpnp[steps].ptr < 0) { \
				 if(!ret) \
					ret = RRD_ERR_UNKNOWN_DATA1; \
			   free(rpnp); \
			   return NULL; \
			 } else expr+=length; \
          } \
        }

        match_op(OP_ADD, +)
            match_op(OP_SUB, -)
            match_op(OP_MUL, *)
            match_op(OP_DIV, /)
            match_op(OP_MOD, %)
            match_op(OP_SIN, SIN)
            match_op(OP_COS, COS)
            match_op(OP_LOG, LOG)
            match_op(OP_FLOOR, FLOOR)
            match_op(OP_CEIL, CEIL)
            match_op(OP_EXP, EXP)
            match_op(OP_DUP, DUP)
            match_op(OP_EXC, EXC)
            match_op(OP_POP, POP)
            match_op(OP_LTIME, LTIME)
            match_op(OP_LT, LT)
            match_op(OP_LE, LE)
            match_op(OP_GT, GT)
            match_op(OP_GE, GE)
            match_op(OP_EQ, EQ)
            match_op(OP_IF, IF)
            match_op(OP_MIN, MIN)
            match_op(OP_MAX, MAX)
            match_op(OP_LIMIT, LIMIT)
            /* order is important here ! .. match longest first */
            match_op(OP_UNKN, UNKN)
            match_op(OP_UN, UN)
            match_op(OP_NEGINF, NEGINF)
            match_op(OP_NE, NE)
            match_op(OP_COUNT, COUNT)
            match_op_param(OP_PREV_OTHER, PREV)
            match_op(OP_PREV, PREV)
            match_op(OP_INF, INF)
            match_op(OP_ISINF, ISINF)
            match_op(OP_NOW, NOW)
            match_op(OP_TIME, TIME)
            match_op(OP_ATAN2, ATAN2)
            match_op(OP_ATAN, ATAN)
            match_op(OP_SQRT, SQRT)
            match_op(OP_SORT, SORT)
            match_op(OP_REV, REV)
            match_op(OP_TREND, TREND)
            match_op(OP_TRENDNAN, TRENDNAN)
            match_op(OP_PREDICT, PREDICT)
            match_op(OP_PREDICTSIGMA, PREDICTSIGMA)
            match_op(OP_RAD2DEG, RAD2DEG)
            match_op(OP_DEG2RAD, DEG2RAD)
            match_op(OP_AVG, AVG)
            match_op(OP_ABS, ABS)
            match_op(OP_ADDNAN, ADDNAN)
            match_op(OP_MINNAN, MINNAN)
            match_op(OP_MAXNAN, MAXNAN)
#undef match_op
            else if ((sscanf(expr, DEF_NAM_FMT "%n", vname, &pos) == 1)
                     && ((rpnp[steps].ptr = (*lookup) (key_hash, vname)) !=
                         -1)) {
            rpnp[steps].op = OP_VARIABLE;
            expr += pos;
        }

        else {
            setlocale(LC_NUMERIC, old_locale);
            free(rpnp);
            return NULL;
        }

        if (*expr == 0)
            break;
        if (*expr == ',')
            expr++;
        else {
            setlocale(LC_NUMERIC, old_locale);
            free(rpnp);
            return NULL;
        }
    }
    rpnp[steps + 1].op = OP_END;
    setlocale(LC_NUMERIC, old_locale);
    return rpnp;
}

void rpnstack_init( rpnstack_t *rpnstack) {
    rpnstack->s = NULL;
    rpnstack->dc_stacksize = 0;
    rpnstack->dc_stackblock = 100;
}

void rpnstack_free( rpnstack_t *rpnstack) {
    if (rpnstack->s != NULL)
        free(rpnstack->s);
    rpnstack->dc_stacksize = 0;
}

static int rpn_compare_double( const void *x, const void *y) {
    double    diff = *((const double *) x) - *((const double *) y);

    return (diff < 0) ? -1 : (diff > 0) ? 1 : 0;
}

/* rpn_calc: run the RPN calculator; also performs variable substitution;
 * moved and modified from data_calc() originally included in rrd_graph.c 
 * arguments:
 * rpnp : an array of RPN operators (including variable references)
 * rpnstack : the initialized stack
 * data_idx : when data_idx is a multiple of rpnp.step, the rpnp.data pointer
 *   is advanced by rpnp.ds_cnt; used only for variable substitution
 * output : an array of output values; OP_PREV assumes this array contains
 *   the "previous" value at index position output_idx-1; the definition of
 *   "previous" depends on the calling environment
 * output_idx : an index into the output array in which to store the output
 *   of the RPN calculator
 * returns: -1 if the computation failed (also calls rrd_set_error)
 *           0 on success
 */
short rpn_calc( rpnp_t *rpnp, rpnstack_t *rpnstack, long data_idx,
    rrd_value_t *output, int output_idx) {
    int       rpi;
    long      stptr = -1;

    /* process each op from the rpn in turn */
    for (rpi = 0; rpnp[rpi].op != OP_END; rpi++) {
        /* allocate or grow the stack */
        if (stptr + 5 > rpnstack->dc_stacksize) {
            /* could move this to a separate function */
            rpnstack->dc_stacksize += rpnstack->dc_stackblock;
            rpnstack->s = (double*)rrd_realloc(rpnstack->s,
                                      (rpnstack->dc_stacksize) *
                                      sizeof(*(rpnstack->s)));
            if (rpnstack->s == NULL) {
                return -RRD_ERR_STACK;
            }
        }
#define stackunderflow(MINSIZE)				\
	if(stptr<MINSIZE){				\
         return -RRD_ERR_STACK1;		\
	}

        switch (rpnp[rpi].op) {
        case OP_NUMBER:
            rpnstack->s[++stptr] = rpnp[rpi].val;
            break;
        case OP_VARIABLE:
        case OP_PREV_OTHER:
            /* Sanity check: VDEFs shouldn't make it here */
            if (rpnp[rpi].ds_cnt == 0) {
                return -RRD_ERR_ABORT;
            } else {
                /* make sure we pull the correct value from
                 * the *.data array. Adjust the pointer into
                 * the array acordingly. Advance the ptr one
                 * row in the rra (skip over non-relevant
                 * data sources)
                 */
                if (rpnp[rpi].op == OP_VARIABLE) {
                    rpnstack->s[++stptr] = *(rpnp[rpi].data);
                } else {
                    if ((output_idx) <= 0) {
                        rpnstack->s[++stptr] = DNAN;
                    } else {
                        rpnstack->s[++stptr] =
                            *(rpnp[rpi].data - rpnp[rpi].ds_cnt);
                    }

                }
                if (data_idx % rpnp[rpi].step == 0) {
                    rpnp[rpi].data += rpnp[rpi].ds_cnt;
                }
            }
            break;
        case OP_COUNT:
            rpnstack->s[++stptr] = (output_idx + 1);    /* Note: Counter starts at 1 */
            break;
        case OP_PREV:
            if ((output_idx) <= 0) {
                rpnstack->s[++stptr] = DNAN;
            } else {
                rpnstack->s[++stptr] = output[output_idx - 1];
            }
            break;
        case OP_UNKN:
            rpnstack->s[++stptr] = DNAN;
            break;
        case OP_INF:
            rpnstack->s[++stptr] = DINF;
            break;
        case OP_NEGINF:
            rpnstack->s[++stptr] = -DINF;
            break;
        case OP_NOW:
            rpnstack->s[++stptr] = (double) time(NULL);
            break;
        case OP_TIME:
            /* HACK: this relies on the data_idx being the time,
             ** which the within-function scope is unaware of */
            rpnstack->s[++stptr] = (double) data_idx;
            break;
        case OP_LTIME:
            rpnstack->s[++stptr] =
                (double) tzoffset(data_idx) + (double) data_idx;
            break;
        case OP_ADD:
            stackunderflow(1);
            rpnstack->s[stptr - 1] = rpnstack->s[stptr - 1]
                + rpnstack->s[stptr];
            stptr--;
            break;
        case OP_ADDNAN:
            stackunderflow(1);
            if (isnan(rpnstack->s[stptr - 1])) {
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            } else if (isnan(rpnstack->s[stptr])) {
                /* NOOP */
                /* rpnstack->s[stptr - 1] = rpnstack->s[stptr - 1]; */
            } else {
                rpnstack->s[stptr - 1] = rpnstack->s[stptr - 1]
                    + rpnstack->s[stptr];
            }

            stptr--;
            break;
        case OP_SUB:
            stackunderflow(1);
            rpnstack->s[stptr - 1] = rpnstack->s[stptr - 1]
                - rpnstack->s[stptr];
            stptr--;
            break;
        case OP_MUL:
            stackunderflow(1);
            rpnstack->s[stptr - 1] = (rpnstack->s[stptr - 1])
                * (rpnstack->s[stptr]);
            stptr--;
            break;
        case OP_DIV:
            stackunderflow(1);
            rpnstack->s[stptr - 1] = rpnstack->s[stptr - 1]
                / rpnstack->s[stptr];
            stptr--;
            break;
        case OP_MOD:
            stackunderflow(1);
            rpnstack->s[stptr - 1] = fmod(rpnstack->s[stptr - 1]
                                          , rpnstack->s[stptr]);
            stptr--;
            break;
        case OP_SIN:
            stackunderflow(0);
            rpnstack->s[stptr] = sin(rpnstack->s[stptr]);
            break;
        case OP_ATAN:
            stackunderflow(0);
            rpnstack->s[stptr] = atan(rpnstack->s[stptr]);
            break;
        case OP_RAD2DEG:
            stackunderflow(0);
            rpnstack->s[stptr] = 57.29577951 * rpnstack->s[stptr];
            break;
        case OP_DEG2RAD:
            stackunderflow(0);
            rpnstack->s[stptr] = 0.0174532952 * rpnstack->s[stptr];
            break;
        case OP_ATAN2:
            stackunderflow(1);
            rpnstack->s[stptr - 1] = atan2(rpnstack->s[stptr - 1],
                                           rpnstack->s[stptr]);
            stptr--;
            break;
        case OP_COS:
            stackunderflow(0);
            rpnstack->s[stptr] = cos(rpnstack->s[stptr]);
            break;
        case OP_CEIL:
            stackunderflow(0);
            rpnstack->s[stptr] = ceil(rpnstack->s[stptr]);
            break;
        case OP_FLOOR:
            stackunderflow(0);
            rpnstack->s[stptr] = floor(rpnstack->s[stptr]);
            break;
        case OP_LOG:
            stackunderflow(0);
            rpnstack->s[stptr] = log(rpnstack->s[stptr]);
            break;
        case OP_DUP:
            stackunderflow(0);
            rpnstack->s[stptr + 1] = rpnstack->s[stptr];
            stptr++;
            break;
        case OP_POP:
            stackunderflow(0);
            stptr--;
            break;
        case OP_EXC:
            stackunderflow(1);
            {
                double    dummy;

                dummy = rpnstack->s[stptr];
                rpnstack->s[stptr] = rpnstack->s[stptr - 1];
                rpnstack->s[stptr - 1] = dummy;
            }
            break;
        case OP_EXP:
            stackunderflow(0);
            rpnstack->s[stptr] = exp(rpnstack->s[stptr]);
            break;
        case OP_LT:
            stackunderflow(1);
            if (isnan(rpnstack->s[stptr - 1]));
            else if (isnan(rpnstack->s[stptr]))
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            else
                rpnstack->s[stptr - 1] = rpnstack->s[stptr - 1] <
                    rpnstack->s[stptr] ? 1.0 : 0.0;
            stptr--;
            break;
        case OP_LE:
            stackunderflow(1);
            if (isnan(rpnstack->s[stptr - 1]));
            else if (isnan(rpnstack->s[stptr]))
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            else
                rpnstack->s[stptr - 1] = rpnstack->s[stptr - 1] <=
                    rpnstack->s[stptr] ? 1.0 : 0.0;
            stptr--;
            break;
        case OP_GT:
            stackunderflow(1);
            if (isnan(rpnstack->s[stptr - 1]));
            else if (isnan(rpnstack->s[stptr]))
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            else
                rpnstack->s[stptr - 1] = rpnstack->s[stptr - 1] >
                    rpnstack->s[stptr] ? 1.0 : 0.0;
            stptr--;
            break;
        case OP_GE:
            stackunderflow(1);
            if (isnan(rpnstack->s[stptr - 1]));
            else if (isnan(rpnstack->s[stptr]))
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            else
                rpnstack->s[stptr - 1] = rpnstack->s[stptr - 1] >=
                    rpnstack->s[stptr] ? 1.0 : 0.0;
            stptr--;
            break;
        case OP_NE:
            stackunderflow(1);
            if (isnan(rpnstack->s[stptr - 1]));
            else if (isnan(rpnstack->s[stptr]))
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            else
                rpnstack->s[stptr - 1] = rpnstack->s[stptr - 1] ==
                    rpnstack->s[stptr] ? 0.0 : 1.0;
            stptr--;
            break;
        case OP_EQ:
            stackunderflow(1);
            if (isnan(rpnstack->s[stptr - 1]));
            else if (isnan(rpnstack->s[stptr]))
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            else
                rpnstack->s[stptr - 1] = rpnstack->s[stptr - 1] ==
                    rpnstack->s[stptr] ? 1.0 : 0.0;
            stptr--;
            break;
        case OP_IF:
            stackunderflow(2);
            rpnstack->s[stptr - 2] = (isnan(rpnstack->s[stptr - 2])
                                      || rpnstack->s[stptr - 2] ==
                                      0.0) ? rpnstack->s[stptr] : rpnstack->
                s[stptr - 1];
            stptr--;
            stptr--;
            break;
        case OP_MIN:
            stackunderflow(1);
            if (isnan(rpnstack->s[stptr - 1]));
            else if (isnan(rpnstack->s[stptr]))
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            else if (rpnstack->s[stptr - 1] > rpnstack->s[stptr])
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            stptr--;
            break;
        case OP_MINNAN:
            stackunderflow(1);
            if (isnan(rpnstack->s[stptr - 1]))
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            else if (isnan(rpnstack->s[stptr]));
            else if (rpnstack->s[stptr - 1] > rpnstack->s[stptr])
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            stptr--;
            break;
        case OP_MAX:
            stackunderflow(1);
            if (isnan(rpnstack->s[stptr - 1]));
            else if (isnan(rpnstack->s[stptr]))
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            else if (rpnstack->s[stptr - 1] < rpnstack->s[stptr])
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            stptr--;
            break;
        case OP_MAXNAN:
            stackunderflow(1);
            if (isnan(rpnstack->s[stptr - 1]))
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            else if (isnan(rpnstack->s[stptr]));
            else if (rpnstack->s[stptr - 1] < rpnstack->s[stptr])
                rpnstack->s[stptr - 1] = rpnstack->s[stptr];
            stptr--;
            break;
        case OP_LIMIT:
            stackunderflow(2);
            if (isnan(rpnstack->s[stptr - 2]));
            else if (isnan(rpnstack->s[stptr - 1]))
                rpnstack->s[stptr - 2] = rpnstack->s[stptr - 1];
            else if (isnan(rpnstack->s[stptr]))
                rpnstack->s[stptr - 2] = rpnstack->s[stptr];
            else if (rpnstack->s[stptr - 2] < rpnstack->s[stptr - 1])
                rpnstack->s[stptr - 2] = DNAN;
            else if (rpnstack->s[stptr - 2] > rpnstack->s[stptr])
                rpnstack->s[stptr - 2] = DNAN;
            stptr -= 2;
            break;
        case OP_UN:
            stackunderflow(0);
            rpnstack->s[stptr] = isnan(rpnstack->s[stptr]) ? 1.0 : 0.0;
            break;
        case OP_ISINF:
            stackunderflow(0);
            rpnstack->s[stptr] = isinf(rpnstack->s[stptr]) ? 1.0 : 0.0;
            break;
        case OP_SQRT:
            stackunderflow(0);
            rpnstack->s[stptr] = sqrt(rpnstack->s[stptr]);
            break;
        case OP_SORT:
            stackunderflow(0);
            {
                int       spn = (int) rpnstack->s[stptr--];

                stackunderflow(spn - 1);
                qsort(rpnstack->s + stptr - spn + 1, spn, sizeof(double),
                      rpn_compare_double);
            }
            break;
        case OP_REV:
            stackunderflow(0);
            {
                int       spn = (int) rpnstack->s[stptr--];
                double   *p, *q;

                stackunderflow(spn - 1);

                p = rpnstack->s + stptr - spn + 1;
                q = rpnstack->s + stptr;
                while (p < q) {
                    double    x = *q;

                    *q-- = *p;
                    *p++ = x;
                }
            }
            break;
        case OP_PREDICT:
        case OP_PREDICTSIGMA:
            stackunderflow(2);
	    {
		/* the local averaging window (similar to trend, but better here, as we get better statistics thru numbers)*/
		int   locstepsize = rpnstack->s[--stptr];
		/* the number of shifts and range-checking*/
		int     shifts = rpnstack->s[--stptr];
                stackunderflow(shifts);
		// handle negative shifts special
		if (shifts<0) {
		    stptr--;
		} else {
		    stptr-=shifts;
		}
		/* the real calculation */
		double val=DNAN;
		/* the info on the datasource */
		time_t  dsstep = (time_t) rpnp[rpi - 1].step;
		int    dscount = rpnp[rpi - 1].ds_cnt;
		int   locstep = (int)ceil((float)locstepsize/(float)dsstep);

		/* the sums */
                double    sum = 0;
		double    sum2 = 0;
                int       count = 0;
		/* now loop for each position */
		int doshifts=shifts;
		if (shifts<0) { doshifts=-shifts; }
		for(int loop=0;loop<doshifts;loop++) {
		    /* calculate shift step */
		    int shiftstep=1;
		    if (shifts<0) {
			shiftstep = loop*rpnstack->s[stptr];
		    } else { 
			shiftstep = rpnstack->s[stptr+loop]; 
		    }
		    if(shiftstep <0) {
			return -RRD_ERR_ALLOW;
		    }
		    shiftstep=(int)ceil((float)shiftstep/(float)dsstep);
		    /* loop all local shifts */
		    for(int i=0;i<=locstep;i++) {
			/* now calculate offset into data-array - relative to output_idx*/
			int offset=shiftstep+i;
			/* and process if we have index 0 of above */
			if ((offset>=0)&&(offset<output_idx)) {
			    /* get the value */
			    val =rpnp[rpi - 1].data[-dscount * offset];
			    /* and handle the non NAN case only*/
			    if (! isnan(val)) {
				sum+=val;
				sum2+=val*val;
				count++;
			    }
			}
		    }
		}
		/* do the final calculations */
		val=DNAN;
		if (rpnp[rpi].op == OP_PREDICT) {  /* the average */
		    if (count>0) {
			val = sum/(double)count;
		    } 
		} else {
		    if (count>1) { /* the sigma case */
			val=count*sum2-sum*sum;
			if (val<0) {
			    val=DNAN;
			} else {
			    val=sqrt(val/((float)count*((float)count-1.0)));
			}
		    }
		}
		rpnstack->s[stptr] = val;
	    }
            break;
        case OP_TREND:
        case OP_TRENDNAN:
            stackunderflow(1);
            if ((rpi < 2) || (rpnp[rpi - 2].op != OP_VARIABLE)) {
                return -RRD_ERR_ARG12;
            } else {
                time_t    dur = (time_t) rpnstack->s[stptr];
                time_t    step = (time_t) rpnp[rpi - 2].step;

                if (output_idx + 1 >= (int) ceil((float) dur / (float) step)) {
                    int       ignorenan = (rpnp[rpi].op == OP_TREND);
                    double    accum = 0.0;
                    int       i = -1; /* pick the current entries, not the next one
                                         as the data pointer has already been forwarded
                                         when the OP_VARIABLE was processed */
                    int       count = 0;

                    do {
                        double    val =
                            rpnp[rpi - 2].data[rpnp[rpi - 2].ds_cnt * i--];
                        if (ignorenan || !isnan(val)) {
                            accum += val;
                            ++count;
                        }

                        dur -= step;
                    } while (dur > 0);

                    rpnstack->s[--stptr] =
                        (count == 0) ? DNAN : (accum / count);
                } else
                    rpnstack->s[--stptr] = DNAN;
            }
            break;
        case OP_AVG:
            stackunderflow(0);
            {
                int       i = (int) rpnstack->s[stptr--];
                double    sum = 0;
                int       count = 0;

                stackunderflow(i - 1);
                while (i > 0) {
                    double    val = rpnstack->s[stptr--];

                    i--;
                    if (isnan(val)) {
                        continue;
                    }
                    count++;
                    sum += val;
                }
                /* now push the result back on stack */
                if (count > 0) {
                    rpnstack->s[++stptr] = sum / count;
                } else {
                    rpnstack->s[++stptr] = DNAN;
                }
            }
            break;
        case OP_ABS:
            stackunderflow(0);
            rpnstack->s[stptr] = fabs(rpnstack->s[stptr]);
            break;
        case OP_END:
            break;
        }
#undef stackunderflow
    }
    if (stptr != 0) {
        return -RRD_ERR_STACK2;
    }

    output[output_idx] = rpnstack->s[0];
    return 0;
}

/* figure out what the local timezone offset for any point in
   time was. Return it in seconds */
int tzoffset(
    time_t now)
{
    int       gm_sec, gm_min, gm_hour, gm_yday, gm_year,
        l_sec, l_min, l_hour, l_yday, l_year;
    struct tm t;
    int       off;

    gmtime_r(&now, &t);
    gm_sec = t.tm_sec;
    gm_min = t.tm_min;
    gm_hour = t.tm_hour;
    gm_yday = t.tm_yday;
    gm_year = t.tm_year;
    localtime_r(&now, &t);
    l_sec = t.tm_sec;
    l_min = t.tm_min;
    l_hour = t.tm_hour;
    l_yday = t.tm_yday;
    l_year = t.tm_year;
    off =
        (l_sec - gm_sec) + (l_min - gm_min) * 60 + (l_hour - gm_hour) * 3600;
    if (l_yday > gm_yday || l_year > gm_year) {
        off += 24 * 3600;
    } else if (l_yday < gm_yday || l_year < gm_year) {
        off -= 24 * 3600;
    }
    return off;
}
