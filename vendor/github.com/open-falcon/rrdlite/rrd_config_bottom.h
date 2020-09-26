#ifndef RRD_CONFIG_BOTTOM_H
#define RRD_CONFIG_BOTTOM_H

/* make sure that we pickup the correct stuff from all headers */
#ifdef HAVE_FEATURES_H
# ifdef _XOPEN_SOURCE
#   undef _XOPEN_SOURCE
# endif
# ifdef _BSD_SOURCE
#  undef _BSD_SOURCE
# endif
# define _XOPEN_SOURCE 600
# define _BSD_SOURCE 1
# include <features.h>
#endif

/* FreeBSD 4.8 wants this included BEFORE sys/types.h */
#ifdef HAVE_SYS_MMAN_H
# include <sys/mman.h>
#endif

#ifdef HAVE_SYS_TYPES_H
# include <sys/types.h>
#endif

#ifdef HAVE_SYS_PARAM_H
# include <sys/param.h>
#endif
#ifndef MAXPATH
# ifdef PATH_MAX
#  define MAXPATH PATH_MAX
# endif
#endif
#ifndef MAXPATH
/* else try the BSD variant */
# ifdef MAXPATHLEN
#  define MAXPATH MAXPATHLEN
# endif
#endif

#ifdef HAVE_ERRNO_H
# include <errno.h>
#endif

#if !defined HAVE_MADVISE && defined HAVE_POSIX_MADVISE
/* use posix_madvise family */
# define madvise posix_madvise
# define MADV_NORMAL POSIX_MADV_NORMAL
# define MADV_RANDOM POSIX_MADV_RANDOM
# define MADV_SEQUENTIAL POSIX_MADV_SEQUENTIAL
# define MADV_WILLNEED POSIX_MADV_WILLNEED
# define MADV_DONTNEED POSIX_MADV_DONTNEED
#endif
#if defined HAVE_MADVISE || defined HAVE_POSIX_MADVISE
# define USE_MADVISE 1
#endif

#ifdef HAVE_SYS_STAT_H
# include <sys/stat.h>
#endif

#ifdef HAVE_FCNTL_H
#include <fcntl.h>
#endif

#ifdef HAVE_UNISTD_H
# include <unistd.h>
#endif

#ifdef TIME_WITH_SYS_TIME
# include <sys/time.h>
# include <time.h>
#else
# ifdef HAVE_SYS_TIME_H
#  include <sys/time.h>
# else
#  include <time.h>
# endif
#endif

#ifdef HAVE_SYS_TIMES_H
# include <sys/times.h>
#endif

#ifdef HAVE_SYS_RESOURCE_H
# include <sys/resource.h>
#if (defined(__svr4__) && defined(__sun__))
/* Solaris headers (pre 2.6) do not have a getrusage prototype. 
   Use this instead. */
extern int getrusage(int, struct rusage *);
#endif /* __svr4__ && __sun__ */
#endif


/* define strrchr, strchr and memcpy, memmove in terms of bsd funcs
   make sure you are NOT using bcopy, index or rindex in the code */
      
#ifdef STDC_HEADERS
# include <string.h>
#else
# ifndef HAVE_STRCHR
#  define strchr index
#  define strrchr rindex
# endif
char *strchr (), *strrchr ();
# ifndef HAVE_MEMMOVE
#  define memcpy(d, s, n) bcopy ((s), (d), (n))
#  define memmove(d, s, n) bcopy ((s), (d), (n))
# endif
#endif

#ifdef NO_NULL_REALLOC
# define rrd_realloc(a,b) ( (a) == NULL ? malloc( (b) ) : realloc( (a) , (b) ))
#else
# define rrd_realloc(a,b) realloc((a), (b))
#endif

#ifdef HAVE_STDIO_H
# include <stdio.h>
#endif

#ifdef HAVE_STDLIB_H
# include <stdlib.h>
#endif

#ifdef HAVE_CTYPE_H
# include <ctype.h>
#endif

#ifdef HAVE_DIRENT_H
# include <dirent.h>
# define NAMLEN(dirent) strlen((dirent)->d_name)
#else
# define dirent direct
# define NAMLEN(dirent) (dirent)->d_namlen
# ifdef HAVE_SYS_NDIR_H
#  include <sys/ndir.h>
# endif
# ifdef HAVE_SYS_DIR_H
#  include <sys/dir.h>
# endif
# ifdef HAVE_NDIR_H
#  include <ndir.h>
# endif
#endif

#ifdef MUST_DISABLE_SIGFPE
# include <signal.h>
#endif

#ifdef MUST_DISABLE_FPMASK
# include <floatingpoint.h>
#endif


#ifdef HAVE_MATH_H
# include <math.h>
#endif

#ifdef HAVE_FLOAT_H
# include <float.h>
#endif

#ifdef HAVE_IEEEFP_H
# include <ieeefp.h>
#endif

#ifdef HAVE_FP_CLASS_H
# include <fp_class.h>
#endif

/* for Solaris */
#if (! defined(HAVE_ISINF) && defined(HAVE_FPCLASS)) 
# define HAVE_ISINF 1
# ifdef isinf
#  undef isinf
# endif
# define isinf(a) (fpclass(a) == FP_NINF || fpclass(a) == FP_PINF)
#endif

/* solaris 8/9 has rint but not round */
#if (! defined(HAVE_ROUND) && defined(HAVE_RINT))
# define round rint
#endif

/* solaris 10 it defines isnan such that only forte can compile it ... bad bad  */
#if (defined(HAVE_ISNAN) && defined(isnan) && defined(HAVE_FPCLASS))
#  undef isnan
#  define isnan(a) (fpclass(a) == FP_SNAN || fpclass(a) == FP_QNAN)
#endif

/* for OSF1 Digital Unix */
#if (! defined(HAVE_ISINF) && defined(HAVE_FP_CLASS) && defined(HAVE_FP_CLASS_H))
#  define HAVE_ISINF 1
#  define isinf(a) (fp_class(a) == FP_NEG_INF || fp_class(a) == FP_POS_INF)
#endif

#if (! defined(HAVE_ISINF) && defined(HAVE_FPCLASSIFY) && defined(FP_PLUS_INF) && defined(FP_MINUS_INF))
#  define HAVE_ISINF 1
#  define isinf(a) (fpclassify(a) == FP_MINUS_INF || fpclassify(a) == FP_PLUS_INF)
#endif

#if (! defined(HAVE_ISINF) && defined(HAVE_FPCLASSIFY) && defined(FP_INFINITE))
#  define HAVE_ISINF 1
#  define isinf(a) (fpclassify(a) == FP_INFINITE)
#endif

/* for AIX */
#if (! defined(HAVE_ISINF) && defined(HAVE_CLASS))
#  define HAVE_ISINF 1
#  define isinf(a) (class(a) == FP_MINUS_INF || class(a) == FP_PLUS_INF)
#endif

#if (! defined (HAVE_FINITE) && defined (HAVE_ISFINITE))
#  define HAVE_FINITE 1
#  define finite(a) isfinite(a)
#endif

#if (! defined(HAVE_FINITE) && defined(HAVE_ISNAN) && defined(HAVE_ISINF))
#  define HAVE_FINITE 1
#  define finite(a) (! isnan(a) && ! isinf(a))
#endif

#ifndef HAVE_FINITE
#error "Can't compile without finite function"
#endif

#ifndef HAVE_ISINF
#error "Can't compile without isinf function"
#endif

#if (! defined(HAVE_FDATASYNC) && defined(HAVE_FSYNC))
#define fdatasync fsync
#endif

#if (!defined(HAVE_FDATASYNC) && !defined(HAVE_FSYNC))
#error "Can't compile with without fsync and fdatasync"
#endif

#endif /* RRD_CONFIG_BOTTOM_H */

