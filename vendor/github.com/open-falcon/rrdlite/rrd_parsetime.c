/*  
 *  rrd_parsetime.c - parse time for at(1)
 *  Copyright (C) 1993, 1994  Thomas Koenig
 *
 *  modifications for English-language times
 *  Copyright (C) 1993  David Parsons
 *
 *  A lot of modifications and extensions 
 *  (including the new syntax being useful for RRDB)
 *  Copyright (C) 1999  Oleg Cherevko (aka Olwi Deer)
 *
 *  severe structural damage inflicted by Tobi Oetiker in 1999
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions
 * are met:
 * 1. Redistributions of source code must retain the above copyright
 *    notice, this list of conditions and the following disclaimer.
 * 2. The name of the author(s) may not be used to endorse or promote
 *    products derived from this software without specific prior written
 *    permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE AUTHOR(S) ``AS IS'' AND ANY EXPRESS OR
 * IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES
 * OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
 * IN NO EVENT SHALL THE AUTHOR(S) BE LIABLE FOR ANY DIRECT, INDIRECT,
 * INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT
 * NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
 * DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
 * THEORY OF LIABILITY, WETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF
 * THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

/* NOTE: nothing in here is thread-safe!!!! Not even the localtime
   calls ... */

/*
 * The BNF-like specification of the time syntax parsed is below:
 *                                                               
 * As usual, [ X ] means that X is optional, { X } means that X may
 * be either omitted or specified as many times as needed,
 * alternatives are separated by |, brackets are used for grouping.
 * (# marks the beginning of comment that extends to the end of line)
 *
 * TIME-SPECIFICATION ::= TIME-REFERENCE [ OFFSET-SPEC ] |
 *			                   OFFSET-SPEC   |
 *			   ( START | END ) OFFSET-SPEC 
 *
 * TIME-REFERENCE ::= NOW | TIME-OF-DAY-SPEC [ DAY-SPEC-1 ] |
 *                        [ TIME-OF-DAY-SPEC ] DAY-SPEC-2
 *
 * TIME-OF-DAY-SPEC ::= NUMBER (':') NUMBER [am|pm] | # HH:MM
 *                     'noon' | 'midnight' | 'teatime'
 *
 * DAY-SPEC-1 ::= NUMBER '/' NUMBER '/' NUMBER |  # MM/DD/[YY]YY
 *                NUMBER '.' NUMBER '.' NUMBER |  # DD.MM.[YY]YY
 *                NUMBER                          # Seconds since 1970
 *                NUMBER                          # YYYYMMDD
 *
 * DAY-SPEC-2 ::= MONTH-NAME NUMBER [NUMBER] |    # Month DD [YY]YY
 *                'yesterday' | 'today' | 'tomorrow' |
 *                DAY-OF-WEEK
 *
 *
 * OFFSET-SPEC ::= '+'|'-' NUMBER TIME-UNIT { ['+'|'-'] NUMBER TIME-UNIT }
 *
 * TIME-UNIT ::= SECONDS | MINUTES | HOURS |
 *               DAYS | WEEKS | MONTHS | YEARS
 *
 * NOW ::= 'now' | 'n'
 *
 * START ::= 'start' | 's'
 * END   ::= 'end' | 'e'
 *
 * SECONDS ::= 'seconds' | 'second' | 'sec' | 's'
 * MINUTES ::= 'minutes' | 'minute' | 'min' | 'm'
 * HOURS   ::= 'hours' | 'hour' | 'hr' | 'h'
 * DAYS    ::= 'days' | 'day' | 'd'
 * WEEKS   ::= 'weeks' | 'week' | 'wk' | 'w'
 * MONTHS  ::= 'months' | 'month' | 'mon' | 'm'
 * YEARS   ::= 'years' | 'year' | 'yr' | 'y'
 *
 * MONTH-NAME ::= 'jan' | 'january' | 'feb' | 'february' | 'mar' | 'march' |
 *                'apr' | 'april' | 'may' | 'jun' | 'june' | 'jul' | 'july' |
 *                'aug' | 'august' | 'sep' | 'september' | 'oct' | 'october' |
 *		  'nov' | 'november' | 'dec' | 'december'
 *
 * DAY-OF-WEEK ::= 'sunday' | 'sun' | 'monday' | 'mon' | 'tuesday' | 'tue' |
 *                 'wednesday' | 'wed' | 'thursday' | 'thu' | 'friday' | 'fri' |
 *                 'saturday' | 'sat'
 *
 *
 * As you may note, there is an ambiguity with respect to
 * the 'm' time unit (which can mean either minutes or months).
 * To cope with this, code tries to read users mind :) by applying
 * certain heuristics. There are two of them:
 *
 * 1. If 'm' is used in context of (i.e. right after the) years,
 *    months, weeks, or days it is assumed to mean months, while
 *    in the context of hours, minutes, and seconds it means minutes.
 *    (e.g., in -1y6m or +3w1m 'm' means 'months', while in
 *    -3h20m or +5s2m 'm' means 'minutes')
 *
 * 2. Out of context (i.e. right after the '+' or '-' sign) the
 *    meaning of 'm' is guessed from the number it directly follows.
 *    Currently, if the number absolute value is below 25 it is assumed
 *    that 'm' means months, otherwise it is treated as minutes.
 *    (e.g., -25m == -25 minutes, while +24m == +24 months)
 *
 */

/* System Headers */

/* Local headers */

#include <stdarg.h>
#include <stdlib.h>
#include <ctype.h>

#include "rrd_tool.h"

/* Structures and unions */

enum {                  /* symbols */
    MIDNIGHT, NOON, TEATIME,
    PM, AM, YESTERDAY, TODAY, TOMORROW, NOW, START, END, EPOCH,
    SECONDS, MINUTES, HOURS, DAYS, WEEKS, MONTHS, YEARS,
    MONTHS_MINUTES,
    NUMBER, PLUS, MINUS, DOT, COLON, SLASH, ID, JUNK,
    JAN, FEB, MAR, APR, MAY, JUN,
    JUL, AUG, SEP, OCT, NOV, DEC,
    SUN, MON, TUE, WED, THU, FRI, SAT
};

/* the below is for plus_minus() */
#define PREVIOUS_OP	(-1)

/* parse translation table - table driven parsers can be your FRIEND!
 */
struct SpecialToken {
    char     *name;     /* token name */
    int       value;    /* token id */
};
static const struct SpecialToken VariousWords[] = {
    {"midnight", MIDNIGHT}, /* 00:00:00 of today or tomorrow */
    {"noon", NOON},     /* 12:00:00 of today or tomorrow */
    {"teatime", TEATIME},   /* 16:00:00 of today or tomorrow */
    {"am", AM},         /* morning times for 0-12 clock */
    {"pm", PM},         /* evening times for 0-12 clock */
    {"tomorrow", TOMORROW},
    {"yesterday", YESTERDAY},
    {"today", TODAY},
    {"now", NOW},
    {"n", NOW},
    {"start", START},
    {"s", START},
    {"end", END},
    {"e", END},
    {"epoch", EPOCH},

    {"jan", JAN},
    {"feb", FEB},
    {"mar", MAR},
    {"apr", APR},
    {"may", MAY},
    {"jun", JUN},
    {"jul", JUL},
    {"aug", AUG},
    {"sep", SEP},
    {"oct", OCT},
    {"nov", NOV},
    {"dec", DEC},
    {"january", JAN},
    {"february", FEB},
    {"march", MAR},
    {"april", APR},
    {"may", MAY},
    {"june", JUN},
    {"july", JUL},
    {"august", AUG},
    {"september", SEP},
    {"october", OCT},
    {"november", NOV},
    {"december", DEC},
    {"sunday", SUN},
    {"sun", SUN},
    {"monday", MON},
    {"mon", MON},
    {"tuesday", TUE},
    {"tue", TUE},
    {"wednesday", WED},
    {"wed", WED},
    {"thursday", THU},
    {"thu", THU},
    {"friday", FRI},
    {"fri", FRI},
    {"saturday", SAT},
    {"sat", SAT},
    {NULL, 0}           /*** SENTINEL ***/
};

static const struct SpecialToken TimeMultipliers[] = {
    {"second", SECONDS},    /* seconds multiplier */
    {"seconds", SECONDS},   /* (pluralized) */
    {"sec", SECONDS},   /* (generic) */
    {"s", SECONDS},     /* (short generic) */
    {"minute", MINUTES},    /* minutes multiplier */
    {"minutes", MINUTES},   /* (pluralized) */
    {"min", MINUTES},   /* (generic) */
    {"m", MONTHS_MINUTES},  /* (short generic) */
    {"hour", HOURS},    /* hours ... */
    {"hours", HOURS},   /* (pluralized) */
    {"hr", HOURS},      /* (generic) */
    {"h", HOURS},       /* (short generic) */
    {"day", DAYS},      /* days ... */
    {"days", DAYS},     /* (pluralized) */
    {"d", DAYS},        /* (short generic) */
    {"week", WEEKS},    /* week ... */
    {"weeks", WEEKS},   /* (pluralized) */
    {"wk", WEEKS},      /* (generic) */
    {"w", WEEKS},       /* (short generic) */
    {"month", MONTHS},  /* week ... */
    {"months", MONTHS}, /* (pluralized) */
    {"mon", MONTHS},    /* (generic) */
    {"year", YEARS},    /* year ... */
    {"years", YEARS},   /* (pluralized) */
    {"yr", YEARS},      /* (generic) */
    {"y", YEARS},       /* (short generic) */
    {NULL, 0}           /*** SENTINEL ***/
};

/* File scope variables */

/* context dependent list of specials for parser to recognize,
 * required for us to be able distinguish between 'mon' as 'month'
 * and 'mon' as 'monday'
 */
static const struct SpecialToken *Specials;

static const char **scp;    /* scanner - pointer at arglist */
static char scc;        /* scanner - count of remaining arguments */
static const char *sct; /* scanner - next char pointer in current argument */
static int need;        /* scanner - need to advance to next argument */

static char *sc_token = NULL;   /* scanner - token buffer */
static size_t sc_len;   /* scanner - length of token buffer */
static int sc_tokid;    /* scanner - token id */

/* Local functions */
static void EnsureMemFree(
    void);

static void EnsureMemFree(
    void)
{
    if (sc_token) {
        free(sc_token);
        sc_token = NULL;
    }
}

/*
 * A hack to compensate for the lack of the C++ exceptions
 *
 * Every function func that might generate parsing "exception"
 * should return TIME_OK (aka NULL) or pointer to the error message,
 * and should be called like this: try(func(args));
 *
 * if the try is not successful it will reset the token pointer ...
 *
 * [NOTE: when try(...) is used as the only statement in the "if-true"
 *  part of the if statement that also has an "else" part it should be
 *  either enclosed in the curly braces (despite the fact that it looks
 *  like a single statement) or NOT followed by the ";"]
 */
#define try(b)		{ \
			char *_e; \
			if((_e=(b))) \
			  { \
			  EnsureMemFree(); \
			  return _e; \
			  } \
			}

/*
 * The panic() function was used in the original code to die, we redefine
 * it as macro to start the chain of ascending returns that in conjunction
 * with the try(b) above will simulate a sort of "exception handling"
 */

#define panic(e)	{ \
			return (e); \
			}

/*
 * ve() and e() are used to set the return error,
 * the most appropriate use for these is inside panic(...) 
 */
#define MAX_ERR_MSG_LEN	1024
static char errmsg[MAX_ERR_MSG_LEN];

static char *ve(
    char *fmt,
    va_list ap)
{
#ifdef HAVE_VSNPRINTF
    vsnprintf(errmsg, MAX_ERR_MSG_LEN, fmt, ap);
#else
    vsprintf(errmsg, fmt, ap);
#endif
    EnsureMemFree();
    return (errmsg);
}

static char *e(
    char *fmt,
    ...)
{
    char     *err;
    va_list   ap;

    va_start(ap, fmt);
    err = ve(fmt, ap);
    va_end(ap);
    return (err);
}

/* Compare S1 and S2, ignoring case, returning less than, equal to or
   greater than zero if S1 is lexicographically less than,
   equal to or greater than S2.  -- copied from GNU libc*/
static int mystrcasecmp(
    const char *s1,
    const char *s2)
{
    const unsigned char *p1 = (const unsigned char *) s1;
    const unsigned char *p2 = (const unsigned char *) s2;
    unsigned char c1, c2;

    if (p1 == p2)
        return 0;

    do {
        c1 = tolower(*p1++);
        c2 = tolower(*p2++);
        if (c1 == '\0')
            break;
    }
    while (c1 == c2);

    return c1 - c2;
}

/*
 * parse a token, checking if it's something special to us
 */
static int parse_token(
    char *arg)
{
    int       i;

    for (i = 0; Specials[i].name != NULL; i++)
        if (mystrcasecmp(Specials[i].name, arg) == 0)
            return sc_tokid = Specials[i].value;

    /* not special - must be some random id */
    return sc_tokid = ID;
}                       /* parse_token */



/*
 * init_scanner() sets up the scanner to eat arguments
 */
static char *init_scanner(
    int argc,
    const char **argv)
{
    scp = argv;
    scc = argc;
    need = 1;
    sc_len = 1;
    while (argc-- > 0)
        sc_len += strlen(*argv++);

    sc_token = (char *) malloc(sc_len * sizeof(char));
    if (sc_token == NULL)
        return "Failed to allocate memory";
    return TIME_OK;
}                       /* init_scanner */

/*
 * token() fetches a token from the input stream
 */
static int token(
    void)
{
    int       idx;

    while (1) {
        memset(sc_token, '\0', sc_len);
        sc_tokid = EOF;
        idx = 0;

        /* if we need to read another argument, walk along the argument list;
         * when we fall off the arglist, we'll just return EOF forever
         */
        if (need) {
            if (scc < 1)
                return sc_tokid;
            sct = *scp;
            scp++;
            scc--;
            need = 0;
        }
        /* eat whitespace now - if we walk off the end of the argument,
         * we'll continue, which puts us up at the top of the while loop
         * to fetch the next argument in
         */
        while (isspace((unsigned char) *sct) || *sct == '_' || *sct == ',')
            ++sct;
        if (!*sct) {
            need = 1;
            continue;
        }

        /* preserve the first character of the new token
         */
        sc_token[0] = *sct++;

        /* then see what it is
         */
        if (isdigit((unsigned char) (sc_token[0]))) {
            while (isdigit((unsigned char) (*sct)))
                sc_token[++idx] = *sct++;
            sc_token[++idx] = '\0';
            return sc_tokid = NUMBER;
        } else if (isalpha((unsigned char) (sc_token[0]))) {
            while (isalpha((unsigned char) (*sct)))
                sc_token[++idx] = *sct++;
            sc_token[++idx] = '\0';
            return parse_token(sc_token);
        } else
            switch (sc_token[0]) {
            case ':':
                return sc_tokid = COLON;
            case '.':
                return sc_tokid = DOT;
            case '+':
                return sc_tokid = PLUS;
            case '-':
                return sc_tokid = MINUS;
            case '/':
                return sc_tokid = SLASH;
            default:
                /*OK, we did not make it ... */
                sct--;
                return sc_tokid = EOF;
            }
    }                   /* while (1) */
}                       /* token */


/* 
 * expect2() gets a token and complains if it's not the token we want
 */
static char *expect2(
    int desired,
    char *complain_fmt,
    ...)
{
    va_list   ap;

    va_start(ap, complain_fmt);
    if (token() != desired) {
        panic(ve(complain_fmt, ap));
    }
    va_end(ap);
    return TIME_OK;

}                       /* expect2 */


/*
 * plus_minus() is used to parse a single NUMBER TIME-UNIT pair
 *              for the OFFSET-SPEC.
 *              It also applies those m-guessing heuristics.
 */
static char *plus_minus(
    rrd_time_value_t * ptv,
    int doop)
{
    static int op = PLUS;
    static int prev_multiplier = -1;
    int       delta;

    if (doop >= 0) {
        op = doop;
        try(expect2
            (NUMBER, "There should be number after '%c'",
             op == PLUS ? '+' : '-'));
        prev_multiplier = -1;   /* reset months-minutes guessing mechanics */
    }
    /* if doop is < 0 then we repeat the previous op
     * with the prefetched number */

    delta = atoi(sc_token);

    if (token() == MONTHS_MINUTES) {
        /* hard job to guess what does that -5m means: -5mon or -5min? */
        switch (prev_multiplier) {
        case DAYS:
        case WEEKS:
        case MONTHS:
        case YEARS:
            sc_tokid = MONTHS;
            break;

        case SECONDS:
        case MINUTES:
        case HOURS:
            sc_tokid = MINUTES;
            break;

        default:
            if (delta < 6)  /* it may be some other value but in the context
                             * of RRD who needs less than 6 min deltas? */
                sc_tokid = MONTHS;
            else
                sc_tokid = MINUTES;
        }
    }
    prev_multiplier = sc_tokid;
    switch (sc_tokid) {
    case YEARS:
        ptv->tm.  tm_year += (
    op == PLUS) ? delta : -delta;

        return TIME_OK;
    case MONTHS:
        ptv->tm.  tm_mon += (
    op == PLUS) ? delta : -delta;

        return TIME_OK;
    case WEEKS:
        delta *= 7;
        /* FALLTHRU */
    case DAYS:
        ptv->tm.  tm_mday += (
    op == PLUS) ? delta : -delta;

        return TIME_OK;
    case HOURS:
        ptv->offset += (op == PLUS) ? delta * 60 * 60 : -delta * 60 * 60;
        return TIME_OK;
    case MINUTES:
        ptv->offset += (op == PLUS) ? delta * 60 : -delta * 60;
        return TIME_OK;
    case SECONDS:
        ptv->offset += (op == PLUS) ? delta : -delta;
        return TIME_OK;
    default:           /*default unit is seconds */
        ptv->offset += (op == PLUS) ? delta : -delta;
        return TIME_OK;
    }
    panic(e("well-known time unit expected after %d", delta));
    /* NORETURN */
    return TIME_OK;     /* to make compiler happy :) */
}                       /* plus_minus */


/*
 * tod() computes the time of day (TIME-OF-DAY-SPEC)
 */
static char *tod(
    rrd_time_value_t * ptv)
{
    int       hour, minute = 0;
    int       tlen;

    /* save token status in  case we must abort */
    int       scc_sv = scc;
    const char *sct_sv = sct;
    int       sc_tokid_sv = sc_tokid;

    tlen = strlen(sc_token);

    /* first pick out the time of day - we assume a HH (COLON|DOT) MM time
     */
    if (tlen > 2) {
        return TIME_OK;
    }

    hour = atoi(sc_token);

    token();
    if (sc_tokid == SLASH || sc_tokid == DOT) {
        /* guess we are looking at a date */
        scc = scc_sv;
        sct = sct_sv;
        sc_tokid = sc_tokid_sv;
        sprintf(sc_token, "%d", hour);
        return TIME_OK;
    }
    if (sc_tokid == COLON) {
        try(expect2(NUMBER,
                    "Parsing HH:MM syntax, expecting MM as number, got none"));
        minute = atoi(sc_token);
        if (minute > 59) {
            panic(e("parsing HH:MM syntax, got MM = %d (>59!)", minute));
        }
        token();
    }

    /* check if an AM or PM specifier was given
     */
    if (sc_tokid == AM || sc_tokid == PM) {
        if (hour > 12) {
            panic(e("there cannot be more than 12 AM or PM hours"));
        }
        if (sc_tokid == PM) {
            if (hour != 12) /* 12:xx PM is 12:xx, not 24:xx */
                hour += 12;
        } else {
            if (hour == 12) /* 12:xx AM is 00:xx, not 12:xx */
                hour = 0;
        }
        token();
    } else if (hour > 23) {
        /* guess it was not a time then ... */
        scc = scc_sv;
        sct = sct_sv;
        sc_tokid = sc_tokid_sv;
        sprintf(sc_token, "%d", hour);
        return TIME_OK;
    }
    ptv->tm.  tm_hour = hour;
    ptv->tm.  tm_min = minute;
    ptv->tm.  tm_sec = 0;

    if (ptv->tm.tm_hour == 24) {
        ptv->tm.  tm_hour = 0;
        ptv->tm.  tm_mday++;
    }
    return TIME_OK;
}                       /* tod */


/*
 * assign_date() assigns a date, adjusting year as appropriate
 */
static char *assign_date(
    rrd_time_value_t * ptv,
    long mday,
    long mon,
    long year)
{
    if (year > 138) {
        if (year > 1970)
            year -= 1900;
        else {
            panic(e("invalid year %d (should be either 00-99 or >1900)",
                    year));
        }
    } else if (year >= 0 && year < 38) {
        year += 100;    /* Allow year 2000-2037 to be specified as   */
    }
    /* 00-37 until the problem of 2038 year will */
    /* arise for unices with 32-bit time_t :)    */
    if (year < 70) {
        panic(e("won't handle dates before epoch (01/01/1970), sorry"));
    }

    ptv->tm.  tm_mday = mday;
    ptv->tm.  tm_mon = mon;
    ptv->tm.  tm_year = year;

    return TIME_OK;
}                       /* assign_date */


/* 
 * day() picks apart DAY-SPEC-[12]
 */
static char *day(
    rrd_time_value_t * ptv)
{
    /* using time_t seems to help portability with 64bit oses */
    time_t    mday = 0, wday, mon, year = ptv->tm.tm_year;

    switch (sc_tokid) {
    case YESTERDAY:
        ptv->tm.  tm_mday--;

        /* FALLTRHU */
    case TODAY:        /* force ourselves to stay in today - no further processing */
        token();
        break;
    case TOMORROW:
        ptv->tm.  tm_mday++;

        token();
        break;

    case JAN:
    case FEB:
    case MAR:
    case APR:
    case MAY:
    case JUN:
    case JUL:
    case AUG:
    case SEP:
    case OCT:
    case NOV:
    case DEC:
        /* do month mday [year]
         */
        mon = (sc_tokid - JAN);
        try(expect2(NUMBER, "the day of the month should follow month name"));
        mday = atol(sc_token);
        if (token() == NUMBER) {
            year = atol(sc_token);
            token();
        } else
            year = ptv->tm.tm_year;

        try(assign_date(ptv, mday, mon, year));
        break;

    case SUN:
    case MON:
    case TUE:
    case WED:
    case THU:
    case FRI:
    case SAT:
        /* do a particular day of the week
         */
        wday = (sc_tokid - SUN);
        ptv->tm.  tm_mday += (
    wday - ptv->tm.tm_wday);

        token();
        break;
        /*
           mday = ptv->tm.tm_mday;
           mday += (wday - ptv->tm.tm_wday);
           ptv->tm.tm_wday = wday;

           try(assign_date(ptv, mday, ptv->tm.tm_mon, ptv->tm.tm_year));
           break;
         */

    case NUMBER:
        /* get numeric <sec since 1970>, MM/DD/[YY]YY, or DD.MM.[YY]YY
         */
        mon = atol(sc_token);
        if (mon > 10 * 365 * 24 * 60 * 60) {
            ptv->tm = *localtime(&mon);

            token();
            break;
        }

        if (mon > 19700101 && mon < 24000101) { /*works between 1900 and 2400 */
            char      cmon[3], cmday[3], cyear[5];

            strncpy(cyear, sc_token, 4);
            cyear[4] = '\0';
            year = atol(cyear);
            strncpy(cmon, &(sc_token[4]), 2);
            cmon[2] = '\0';
            mon = atol(cmon);
            strncpy(cmday, &(sc_token[6]), 2);
            cmday[2] = '\0';
            mday = atol(cmday);
            token();
        } else {
            token();

            if (mon <= 31 && (sc_tokid == SLASH || sc_tokid == DOT)) {
                int       sep;

                sep = sc_tokid;
                try(expect2(NUMBER, "there should be %s number after '%c'",
                            sep == DOT ? "month" : "day",
                            sep == DOT ? '.' : '/'));
                mday = atol(sc_token);
                if (token() == sep) {
                    try(expect2
                        (NUMBER, "there should be year number after '%c'",
                         sep == DOT ? '.' : '/'));
                    year = atol(sc_token);
                    token();
                }

                /* flip months and days for European timing
                 */
                if (sep == DOT) {
                    long      x = mday;

                    mday = mon;
                    mon = x;
                }
            }
        }

        mon--;
        if (mon < 0 || mon > 11) {
            panic(e("did you really mean month %d?", mon + 1));
        }
        if (mday < 1 || mday > 31) {
            panic(e("I'm afraid that %d is not a valid day of the month",
                    mday));
        }
        try(assign_date(ptv, mday, mon, year));
        break;
    }                   /* case */
    return TIME_OK;
}                       /* month */


/* Global functions */


/*
 * rrd_parsetime() is the external interface that takes tspec, parses
 * it and puts the result in the rrd_time_value structure *ptv.
 * It can return either absolute times (these are ensured to be
 * correct) or relative time references that are expected to be
 * added to some absolute time value and then normalized by
 * mktime() The return value is either TIME_OK (aka NULL) or
 * the pointer to the error message in the case of problems
 */
char     *rrd_parsetime(
    const char *tspec,
    rrd_time_value_t * ptv)
{
    time_t    now = time(NULL);
    int       hr = 0;

    /* this MUST be initialized to zero for midnight/noon/teatime */

    Specials = VariousWords;    /* initialize special words context */

    try(init_scanner(1, &tspec));

    /* establish the default time reference */
    ptv->type = ABSOLUTE_TIME;
    ptv->offset = 0;
    ptv->tm = *localtime(&now);
    ptv->tm.  tm_isdst = -1;    /* mk time can figure dst by default ... */

    token();
    switch (sc_tokid) {
    case PLUS:
    case MINUS:
        break;          /* jump to OFFSET-SPEC part */

    case EPOCH:
        ptv->type = RELATIVE_TO_EPOCH;
        goto KeepItRelative;
    case START:
        ptv->type = RELATIVE_TO_START_TIME;
        goto KeepItRelative;
    case END:
        ptv->type = RELATIVE_TO_END_TIME;
      KeepItRelative:
        ptv->tm.  tm_sec = 0;
        ptv->tm.  tm_min = 0;
        ptv->tm.  tm_hour = 0;
        ptv->tm.  tm_mday = 0;
        ptv->tm.  tm_mon = 0;
        ptv->tm.  tm_year = 0;

        /* FALLTHRU */
    case NOW:
    {
        int       time_reference = sc_tokid;

        token();
        if (sc_tokid == PLUS || sc_tokid == MINUS)
            break;
        if (time_reference != NOW) {
            panic(e("'start' or 'end' MUST be followed by +|- offset"));
        } else if (sc_tokid != EOF) {
            panic(e("if 'now' is followed by a token it must be +|- offset"));
        }
    };
        break;

        /* Only absolute time specifications below */
    case NUMBER:
    {
        long      hour_sv = ptv->tm.tm_hour;
        long      year_sv = ptv->tm.tm_year;

        ptv->tm.  tm_hour = 30;
        ptv->tm.  tm_year = 30000;

        try(tod(ptv))
            try(day(ptv))
            if (ptv->tm.tm_hour == 30 && ptv->tm.tm_year != 30000) {
            try(tod(ptv))
        }
        if (ptv->tm.tm_hour == 30) {
            ptv->tm.  tm_hour = hour_sv;
        }
        if (ptv->tm.tm_year == 30000) {
            ptv->tm.  tm_year = year_sv;
        }
    };
        break;
        /* fix month parsing */
    case JAN:
    case FEB:
    case MAR:
    case APR:
    case MAY:
    case JUN:
    case JUL:
    case AUG:
    case SEP:
    case OCT:
    case NOV:
    case DEC:
        try(day(ptv));
        if (sc_tokid != NUMBER)
            break;
        try(tod(ptv))
            break;

        /* evil coding for TEATIME|NOON|MIDNIGHT - we've initialized
         * hr to zero up above, then fall into this case in such a
         * way so we add +12 +4 hours to it for teatime, +12 hours
         * to it for noon, and nothing at all for midnight, then
         * set our rettime to that hour before leaping into the
         * month scanner
         */
    case TEATIME:
        hr += 4;
        /* FALLTHRU */
    case NOON:
        hr += 12;
        /* FALLTHRU */
    case MIDNIGHT:
        /* if (ptv->tm.tm_hour >= hr) {
           ptv->tm.tm_mday++;
           ptv->tm.tm_wday++;
           } *//* shifting does not makes sense here ... noon is noon */
        ptv->tm.  tm_hour = hr;
        ptv->tm.  tm_min = 0;
        ptv->tm.  tm_sec = 0;

        token();
        try(day(ptv));
        break;
    default:
        panic(e("unparsable time: %s%s", sc_token, sct));
        break;
    }                   /* ugly case statement */

    /*
     * the OFFSET-SPEC part
     *
     * (NOTE, the sc_tokid was prefetched for us by the previous code)
     */
    if (sc_tokid == PLUS || sc_tokid == MINUS) {
        Specials = TimeMultipliers; /* switch special words context */
        while (sc_tokid == PLUS || sc_tokid == MINUS || sc_tokid == NUMBER) {
            if (sc_tokid == NUMBER) {
                try(plus_minus(ptv, PREVIOUS_OP));
            } else
                try(plus_minus(ptv, sc_tokid));
            token();    /* We will get EOF eventually but that's OK, since
                           token() will return us as many EOFs as needed */
        }
    }

    /* now we should be at EOF */
    if (sc_tokid != EOF) {
        panic(e("unparsable trailing text: '...%s%s'", sc_token, sct));
    }

    if (ptv->type == ABSOLUTE_TIME)
        if (mktime(&ptv->tm) == -1) {   /* normalize & check */
            /* can happen for "nonexistent" times, e.g. around 3am */
            /* when winter -> summer time correction eats a hour */
            panic(e("the specified time is incorrect (out of range?)"));
        }
    EnsureMemFree();
    return TIME_OK;
}                       /* rrd_parsetime */


int rrd_proc_start_end(
    rrd_time_value_t * start_tv,
    rrd_time_value_t * end_tv,
    time_t *start,
    time_t *end)
{
    if (start_tv->type == RELATIVE_TO_END_TIME &&   /* same as the line above */
        end_tv->type == RELATIVE_TO_START_TIME) {
        return -RRD_ERR_TIME4;
    }

    if (start_tv->type == RELATIVE_TO_START_TIME) {
        return -RRD_ERR_TIME5;
    }

    if (end_tv->type == RELATIVE_TO_END_TIME) {
        return -RRD_ERR_TIME6;
    }

    if (start_tv->type == RELATIVE_TO_END_TIME) {
        struct tm tmtmp;

        *end = mktime(&(end_tv->tm)) + end_tv->offset;
        tmtmp = *localtime(end);    /* reinit end including offset */
        tmtmp.tm_mday += start_tv->tm.tm_mday;
        tmtmp.tm_mon += start_tv->tm.tm_mon;
        tmtmp.tm_year += start_tv->tm.tm_year;

        *start = mktime(&tmtmp) + start_tv->offset;
    } else {
        *start = mktime(&(start_tv->tm)) + start_tv->offset;
    }
    if (end_tv->type == RELATIVE_TO_START_TIME) {
        struct tm tmtmp;

        *start = mktime(&(start_tv->tm)) + start_tv->offset;
        tmtmp = *localtime(start);
        tmtmp.tm_mday += end_tv->tm.tm_mday;
        tmtmp.tm_mon += end_tv->tm.tm_mon;
        tmtmp.tm_year += end_tv->tm.tm_year;

        *end = mktime(&tmtmp) + end_tv->offset;
    } else {
        *end = mktime(&(end_tv->tm)) + end_tv->offset;
    }
    return 0;
}                       /* rrd_proc_start_end */
