/**
 * RRDtool - src/rrd_utils.c
 * Copyright (C) 2009 Kevin Brintnall
 * Copyright (C) 2008 Sebastian Harl
 *
 * This program is free software; you can redistribute it and/or modify it
 * under the terms of the GNU General Public License as published by the
 * Free Software Foundation; only version 2 of the License is applicable.
 *
 * This program is distributed in the hope that it will be useful, but
 * WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
 * General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along
 * with this program; if not, write to the Free Software Foundation, Inc.,
 * 51 Franklin St, Fifth Floor, Boston, MA  02110-1301 USA
 *
 * Authors:
 *   kevin brintnall <kbrint@rufus.net>
 *   Sebastian Harl <sh@tokkee.org>
 **/

#include "rrd_tool.h"

#include <assert.h>
#include <errno.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <sys/stat.h>
#ifndef _MSC_VER
#include <libgen.h>
#include <unistd.h>
#endif
#ifdef WIN32
#	define random() rand()
#	define srandom(x) srand(x)
#	define getpid() 0
#endif /* WIN32 */

#ifndef S_ISDIR
#define S_ISDIR(x) (((x) & S_IFMT) == S_IFDIR)
#endif

/* make sure that the random number generator seeded EXACTLY ONCE */
long rrd_random(void)
{
    static int rand_init = 0;
    if (!rand_init) {
        srandom((unsigned int) time(NULL) + (unsigned int) getpid());
        rand_init++;
    }

    return random();
}

/* rrd_add_ptr: add a pointer to a dynamically sized array of pointers,
 * realloc as necessary.  returns 1 on success, 0 on failure.
 */

int rrd_add_ptr(void ***dest, size_t *dest_size, void *src)
{
    void **temp;

    assert(dest != NULL);

    temp = (void **) rrd_realloc(*dest, (*dest_size+1) * sizeof(*dest));
    if (!temp)
        return 0;

    *dest = temp;
    temp[*dest_size] = src;
    (*dest_size)++;

    return 1;
}

/* like rrd_add_ptr, but calls strdup() on a string first. */
int rrd_add_strdup(char ***dest, size_t *dest_size, char *src)
{
    char *dup_src;
    int add_ok;

    assert(dest != NULL);
    assert(src  != NULL);

    dup_src = strdup(src);
    if (!dup_src)
        return 0;

    add_ok = rrd_add_ptr((void ***)dest, dest_size, (void *)dup_src);
    if (!add_ok)
        free(dup_src);

    return add_ok;
}

void rrd_free_ptrs(void ***src, size_t *cnt)
{
    void **sp;

    assert(src != NULL);
    sp = *src;

    if (sp == NULL)
        return;

    while (*cnt > 0) {
        (*cnt)--;
        free(sp[*cnt]);
    }

    free (sp);
    *src = NULL;
}

/* recursively create the directory named by 'pathname'
 * (similar to "mkdir -p" on the command line) */
int rrd_mkdir_p(const char *pathname, mode_t mode)
{
    struct stat sb;

    char *pathname_copy;
    char *base_dir;

    if ((NULL == pathname) || ('\0' == *pathname)) {
        errno = EINVAL;
        return -1;
    }

    if (0 == stat(pathname, &sb)) {
        if (! S_ISDIR(sb.st_mode)) {
            errno = ENOTDIR;
            return -1;
        }
        return 0;
    }

    /* keep errno as set by stat() */
    if (ENOENT != errno)
        return -1;

    /* dirname might modify its first argument */
    if (NULL == (pathname_copy = strdup(pathname)))
        return -1;

#ifndef _MSC_VER
    /* the data pointedd too by dirname might change too (bsd) */
    if (NULL == (base_dir = strdup(dirname(pathname_copy)))) {
        free(pathname_copy);
        return -1;
    }
#else
    _splitpath(pathname_copy, NULL, base_dir, NULL, NULL);
#endif

    if (0 != rrd_mkdir_p(base_dir, mode)) {
        int orig_errno = errno;
        free(pathname_copy);
#ifndef _MSC_VER
        free(base_dir);
#endif
        errno = orig_errno;
        return -1;
    }

    free(pathname_copy);
#ifndef _MSC_VER
    free(base_dir);
#endif

    /* keep errno as set by mkdir() */
#ifdef _MSC_VER
    if (0 != mkdir(pathname))
        return -1;
#else
    if (0 != mkdir(pathname, mode))
        return -1;
#endif
    return 0;
} /* rrd_mkdir_p */

