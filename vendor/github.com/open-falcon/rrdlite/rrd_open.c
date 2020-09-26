/*****************************************************************************
 * RRDtool 1.4.9  Copyright by Tobi Oetiker, 1997-2014
 *****************************************************************************
 * rrd_open.c  Open an RRD File
 *****************************************************************************
 * $Id$
 *****************************************************************************/

#include "rrd_tool.h"
#include "unused.h"

#ifdef WIN32
#include <stdlib.h>
#include <fcntl.h>
#include <sys/stat.h>
#endif

#ifdef HAVE_BROKEN_MS_ASYNC
#include <sys/types.h>
#include <utime.h>
#endif

#define MEMBLK 8192

#ifdef WIN32
#define	_LK_UNLCK	0	/* Unlock */
#define	_LK_LOCK	1	/* Lock */
#define	_LK_NBLCK	2	/* Non-blocking lock */
#define	_LK_RLCK	3	/* Lock for read only */
#define	_LK_NBRLCK	4	/* Non-blocking lock for read only */


#define	LK_UNLCK	_LK_UNLCK
#define	LK_LOCK		_LK_LOCK
#define	LK_NBLCK	_LK_NBLCK
#define	LK_RLCK		_LK_RLCK
#define	LK_NBRLCK	_LK_NBRLCK
#endif

/* DEBUG 2 prints information obtained via mincore(2) */
#define DEBUG 1
/* do not calculate exact madvise hints but assume 1 page for headers and
 * set DONTNEED for the rest, which is assumed to be data */
/* Avoid calling madvise on areas that were already hinted. May be benefical if
 * your syscalls are very slow */

#ifdef HAVE_MMAP
/* the cast to void* is there to avoid this warning seen on ia64 with certain
   versions of gcc: 'cast increases required alignment of target type'
   */
#define __rrd_read(dst, dst_t, cnt) { \
	size_t wanted = sizeof(dst_t)*(cnt); \
	if (offset + wanted > rrd_file->file_len) { \
		ret = -RRD_ERR_READ3; \
		goto out_nullify_head; \
	} \
	(dst) = (dst_t*)(void*) (data + offset); \
	offset += wanted; \
}
#else
#define __rrd_read(dst, dst_t, cnt) { \
	size_t wanted = sizeof(dst_t)*(cnt); \
	size_t got; \
	if ((dst = (dst_t*)malloc(wanted)) == NULL) { \
		ret = -RRD_ERR_MALLOC6; \
		goto out_nullify_head; \
	} \
	got = read (rrd_simple_file->fd, dst, wanted); \
	if (got != wanted) { \
		ret = -RRD_ERR_READ4; \
		goto out_nullify_head; \
	} \
	offset += got; \
}
#endif

/* get the address of the start of this page */
#if defined USE_MADVISE || defined HAVE_POSIX_FADVISE
#ifndef PAGE_START
#define PAGE_START(addr) ((addr)&(~(_page_size-1)))
#endif
#endif

/* Open a database file, return its header and an open filehandle,
 * positioned to the first cdp in the first rra.
 * In the error path of rrd_open, only rrd_free(&rrd) has to be called
 * before returning an error. Do not call rrd_close upon failure of rrd_open.
 * If creating a new file, the parameter rrd must be initialised with
 * details of the file content.
 * If opening an existing file, then use rrd must be initialised by
 * rrd_init(rrd) prior to invoking rrd_open
 */

rrd_file_t *rrd_open( const char *const file_name, rrd_t *rrd, 
		unsigned rdwr, int *ret_p) {
	unsigned long ui;
	int       flags = 0;
	int       version;
	int       ret = 0;

#ifdef HAVE_MMAP
	ssize_t   _page_size = sysconf(_SC_PAGESIZE);
	char     *data = MAP_FAILED;
#endif
	off_t     offset = 0;
	struct stat statb;
	rrd_file_t *rrd_file = NULL;
	rrd_simple_file_t *rrd_simple_file = NULL;
	size_t     newfile_size = 0;
	size_t header_len, value_cnt, data_len;

	/* Are we creating a new file? */
	if((rdwr & RRD_CREAT) && (rrd->stat_head != NULL)) {
		header_len = rrd_get_header_size(rrd);

		value_cnt = 0;
		for (ui = 0; ui < rrd->stat_head->rra_cnt; ui++)
			value_cnt += rrd->stat_head->ds_cnt * rrd->rra_def[ui].row_cnt;

		data_len = sizeof(rrd_value_t) * value_cnt;

		newfile_size = header_len + data_len;
	}

	rrd_file = (rrd_file_t*)malloc(sizeof(rrd_file_t));
	if (rrd_file == NULL) {
		*ret_p = -RRD_ERR_MALLOC7;
		return NULL;
	}
	memset(rrd_file, 0, sizeof(rrd_file_t));

	rrd_file->pvt = malloc(sizeof(rrd_simple_file_t));
	if(rrd_file->pvt == NULL) {
		*ret_p = -RRD_ERR_MALLOC8;
		return NULL;
	}
	memset(rrd_file->pvt, 0, sizeof(rrd_simple_file_t));
	rrd_simple_file = (rrd_simple_file_t *)rrd_file->pvt;

#ifdef DEBUG
	if ((rdwr & (RRD_READONLY | RRD_READWRITE)) ==
			(RRD_READONLY | RRD_READWRITE)) {
		/* Both READONLY and READWRITE were given, which is invalid.  */
		*ret_p = -RRD_ERR_IO1;
		exit(-1);
	}
#endif

#ifdef HAVE_MMAP
	rrd_simple_file->mm_prot = PROT_READ;
	rrd_simple_file->mm_flags = 0;
#endif

	if (rdwr & RRD_READONLY) {
		flags |= O_RDONLY;
#ifdef HAVE_MMAP
# if !defined(AIX)
		rrd_simple_file->mm_flags = MAP_PRIVATE;
# endif
# ifdef MAP_NORESERVE
		rrd_simple_file->mm_flags |= MAP_NORESERVE;  /* readonly, so no swap backing needed */
# endif
#endif
	} else {
		if (rdwr & RRD_READWRITE) {
			flags |= O_RDWR;
#ifdef HAVE_MMAP 
			rrd_simple_file->mm_flags = MAP_SHARED; 
			rrd_simple_file->mm_prot |= PROT_WRITE; 
#endif 
		}
		if (rdwr & RRD_CREAT) {
			flags |= (O_CREAT | O_TRUNC);
		}
		if (rdwr & RRD_EXCL) {
			flags |= O_EXCL;
		}
	}
	if (rdwr & RRD_READAHEAD) {
#ifdef MAP_POPULATE
		rrd_simple_file->mm_flags |= MAP_POPULATE;   /* populate ptes and data */
#endif
#if defined MAP_NONBLOCK
		rrd_simple_file->mm_flags |= MAP_NONBLOCK;   /* just populate ptes */
#endif
	}
#if defined(_WIN32) && !defined(__CYGWIN__) && !defined(__CYGWIN32__)
	flags |= O_BINARY;
#endif

	if ((rrd_simple_file->fd = open(file_name, flags, 0666)) < 0) {
		ret = -RRD_ERR_OPEN_FILE;
		goto out_free;
	}

#ifdef HAVE_MMAP
#ifdef HAVE_BROKEN_MS_ASYNC
	if (rdwr & RRD_READWRITE) {    
		/* some unices, the files mtime does not get update    
		   on msync MS_ASYNC, in order to help them,     
		   we update the the timestamp at this point.      
		   The thing happens pretty 'close' to the open    
		   call so the chances of a race should be minimal.    

		   Maybe ask your vendor to fix your OS ... */    
		utime(file_name,NULL);  
	}
#endif    
#endif

	/* Better try to avoid seeks as much as possible. stat may be heavy but
	 * many concurrent seeks are even worse.  */
	if (newfile_size == 0 && ((fstat(rrd_simple_file->fd, &statb)) < 0)) {
		ret = -RRD_ERR_STAT_FILE;
		goto out_close;
	}
	if (newfile_size == 0) {
		rrd_file->file_len = statb.st_size;
	} else {
		rrd_file->file_len = newfile_size;
#ifdef HAVE_POSIX_FALLOCATE
		if (posix_fallocate(rrd_simple_file->fd, 0, newfile_size) == 0){
			/* if all  is well we skip the seeking below */            
			goto no_lseek_necessary;        
		}
#endif
		lseek(rrd_simple_file->fd, newfile_size - 1, SEEK_SET);
		if ( write(rrd_simple_file->fd, "\0", 1) == -1){    /* poke */
			ret = -RRD_ERR_WRITE5;
			goto out_close;
		}
		lseek(rrd_simple_file->fd, 0, SEEK_SET);
	}
no_lseek_necessary:
#ifdef HAVE_POSIX_FADVISE
	/* In general we need no read-ahead when dealing with rrd_files.
	   When we stop reading, it is highly unlikely that we start up again.
	   In this manner we actually save time and diskaccess (and buffer cache).
	   Thanks to Dave Plonka for the Idea of using POSIX_FADV_RANDOM here. */
	posix_fadvise(rrd_simple_file->fd, 0, 0, POSIX_FADV_RANDOM);
#endif

#ifdef HAVE_MMAP
#ifndef HAVE_POSIX_FALLOCATE
	/* force allocating the file on the underlaying filesystem to prevent any
	 * future bus error when the filesystem is full and attempting to write
	 * trough the file mapping. Filling the file using memset on the file
	 * mapping can also lead some bus error, so we use the old fashioned
	 * write().
	 */
	if (rdwr & RRD_CREAT) {
		char     buf[4096];
		unsigned i;

		memset(buf, DNAN, sizeof buf);
		lseek(rrd_simple_file->fd, offset, SEEK_SET);

		for (i = 0; i < (newfile_size - 1) / sizeof buf; ++i) {
			if (write(rrd_simple_file->fd, buf, sizeof buf) == -1) {
				ret = -RRD_ERR_WRITE5;
				goto out_close;
			}
		}

		if (write(rrd_simple_file->fd, buf,
					(newfile_size - 1) % sizeof buf) == -1) {
			ret = -RRD_ERR_WRITE5;
			goto out_close;
		}

		lseek(rrd_simple_file->fd, 0, SEEK_SET);
	}
#endif

	data = mmap(0, rrd_file->file_len, 
			rrd_simple_file->mm_prot, rrd_simple_file->mm_flags,
			rrd_simple_file->fd, offset);

	/* lets see if the first read worked */
	if (data == MAP_FAILED) {
		ret = -RRD_ERR_MMAP;
		goto out_close;
	}
	rrd_simple_file->file_start = data;
#endif
	if (rdwr & RRD_CREAT)
		goto out_done;
#ifdef USE_MADVISE
	if (rdwr & RRD_COPY) {
		/* We will read everything in a moment (copying) */
		madvise(data, rrd_file->file_len, MADV_WILLNEED );
		madvise(data, rrd_file->file_len, MADV_SEQUENTIAL );
	} else {
		/* We do not need to read anything in for the moment */
		madvise(data, rrd_file->file_len, MADV_RANDOM);
		/* the stat_head will be needed soonish, so hint accordingly */
		madvise(data, sizeof(stat_head_t), MADV_WILLNEED);
		madvise(data, sizeof(stat_head_t), MADV_RANDOM);
	}
#endif

	__rrd_read(rrd->stat_head, stat_head_t, 1);

	/* lets do some test if we are on track ... */
	if (memcmp(rrd->stat_head->cookie, RRD_COOKIE, sizeof(RRD_COOKIE)) != 0) {
		ret = -RRD_ERR_FILE;
		goto out_nullify_head;
	}

	if (rrd->stat_head->float_cookie != FLOAT_COOKIE) {
		ret = -RRD_ERR_FILE1;
		goto out_nullify_head;
	}

	version = atoi(rrd->stat_head->version);

	if (version > atoi(RRD_VERSION)) {
		ret = -RRD_ERR_FILE2;
		goto out_nullify_head;
	}
#if defined USE_MADVISE
	/* the ds_def will be needed soonish, so hint accordingly */
	madvise(data + PAGE_START(offset),
			sizeof(ds_def_t) * rrd->stat_head->ds_cnt, MADV_WILLNEED);
#endif
	__rrd_read(rrd->ds_def, ds_def_t, rrd->stat_head->ds_cnt);

#if defined USE_MADVISE
	/* the rra_def will be needed soonish, so hint accordingly */
	madvise(data + PAGE_START(offset),
			sizeof(rra_def_t) * rrd->stat_head->rra_cnt, MADV_WILLNEED);
#endif
	__rrd_read(rrd->rra_def, rra_def_t,
			rrd->stat_head->rra_cnt);

	/* handle different format for the live_head */
	if (version < 3) {
		rrd->live_head = (live_head_t *) malloc(sizeof(live_head_t));
		if (rrd->live_head == NULL) {
			ret = -RRD_ERR_MALLOC9;
			goto out_close;
		}
#if defined USE_MADVISE
		/* the live_head will be needed soonish, so hint accordingly */
		madvise(data + PAGE_START(offset), sizeof(time_t), MADV_WILLNEED);
#endif
		__rrd_read(rrd->legacy_last_up, time_t,
				1);

		rrd->live_head->last_up = *rrd->legacy_last_up;
		rrd->live_head->last_up_usec = 0;
	} else {
#if defined USE_MADVISE
		/* the live_head will be needed soonish, so hint accordingly */
		madvise(data + PAGE_START(offset),
				sizeof(live_head_t), MADV_WILLNEED);
#endif
		__rrd_read(rrd->live_head, live_head_t,
				1);
	}
	__rrd_read(rrd->pdp_prep, pdp_prep_t,
			rrd->stat_head->ds_cnt);
	__rrd_read(rrd->cdp_prep, cdp_prep_t,
			rrd->stat_head->rra_cnt * rrd->stat_head->ds_cnt);
	__rrd_read(rrd->rra_ptr, rra_ptr_t,
			rrd->stat_head->rra_cnt);

	rrd_file->header_len = offset;
	rrd_file->pos = offset;

	{
		unsigned long row_cnt = 0;

		for (ui=0; ui<rrd->stat_head->rra_cnt; ui++)
			row_cnt += rrd->rra_def[ui].row_cnt;

		size_t  correct_len = rrd_file->header_len +
			sizeof(rrd_value_t) * row_cnt * rrd->stat_head->ds_cnt;

		if (correct_len > rrd_file->file_len) {
			ret = -RRD_ERR_FILE3;
			goto out_nullify_head;
		}
	}

out_done:
	return (rrd_file);
out_nullify_head:
	rrd->stat_head = NULL;
out_close:
#ifdef HAVE_MMAP
	if (data != MAP_FAILED)
		munmap(data, rrd_file->file_len);
#endif

	close(rrd_simple_file->fd);
out_free:
	free(rrd_file->pvt);
	free(rrd_file);
	*ret_p = ret;
	return NULL;
}


#if defined DEBUG && DEBUG > 1
/* Print list of in-core pages of a the current rrd_file.  */
	static
void mincore_print(
		rrd_file_t *rrd_file,
		char *mark)
{
	rrd_simple_file_t *rrd_simple_file;
	rrd_simple_file = (rrd_simple_file_t *)rrd_file->pvt;
#ifdef HAVE_MMAP
	/* pretty print blocks in core */
	size_t     off;
	unsigned char *vec;
	ssize_t   _page_size = sysconf(_SC_PAGESIZE);

	off = rrd_file->file_len +
		((rrd_file->file_len + _page_size - 1) / _page_size);
	vec = malloc(off);
	if (vec != NULL) {
		memset(vec, 0, off);
		if (mincore(rrd_simple_file->file_start, rrd_file->file_len, vec) == 0) {
			int       prev;
			unsigned  is_in = 0, was_in = 0;

			for (off = 0, prev = 0; off < rrd_file->file_len; ++off) {
				is_in = vec[off] & 1;   /* if lsb set then is core resident */
				if (off == 0)
					was_in = is_in;
				if (was_in != is_in) {
					fprintf(stderr, "%s: %sin core: %p len %ld\n", mark,
							was_in ? "" : "not ", vec + prev, off - prev);
					was_in = is_in;
					prev = off;
				}
			}
			fprintf(stderr,
					"%s: %sin core: %p len %ld\n", mark,
					was_in ? "" : "not ", vec + prev, off - prev);
		} else
			fprintf(stderr, "mincore: %s", rrd_strerror(errno));
	}
#else
	fprintf(stderr, "sorry mincore only works with mmap");
#endif
}
#endif                          /* defined DEBUG && DEBUG > 1 */

/*
 * get exclusive lock to whole file.
 * lock gets removed when we close the file
 *
 * returns 0 on success
 */
int rrd_lock(
		rrd_file_t *rrd_file)
{
	int       rcstat;
	rrd_simple_file_t *rrd_simple_file;
	rrd_simple_file = (rrd_simple_file_t *)rrd_file->pvt;

	{
#if defined(_WIN32) && !defined(__CYGWIN__) && !defined(__CYGWIN32__)
		struct _stat st;

		if (_fstat(rrd_simple_file->fd, &st) == 0) {
			rcstat = _locking(rrd_simple_file->fd, _LK_NBLCK, st.st_size);
		} else {
			rcstat = -1;
		}
#else
		struct flock lock;

		lock.l_type = F_WRLCK;  /* exclusive write lock */
		lock.l_len = 0; /* whole file */
		lock.l_start = 0;   /* start of file */
		lock.l_whence = SEEK_SET;   /* end of file */

		rcstat = fcntl(rrd_simple_file->fd, F_SETLK, &lock);
#endif
	}

	return (rcstat);
}


/* drop cache except for the header and the active pages */
void rrd_dontneed( rrd_file_t *rrd_file, rrd_t *rrd) {
	rrd_simple_file_t *rrd_simple_file = (rrd_simple_file_t *)rrd_file->pvt;
#if defined USE_MADVISE || defined HAVE_POSIX_FADVISE
	size_t dontneed_start;
	size_t rra_start;
	size_t active_block;
	size_t i;
	ssize_t   _page_size = sysconf(_SC_PAGESIZE);

	if (rrd_file == NULL) {
#if defined DEBUG && DEBUG
		fprintf (stderr, "rrd_dontneed: Argument 'rrd_file' is NULL.\n");
#endif
		return;
	}

#if defined DEBUG && DEBUG > 1
	mincore_print(rrd_file, "before");
#endif

	/* ignoring errors from RRDs that are smaller then the file_len+rounding */
	rra_start = rrd_file->header_len;
	dontneed_start = PAGE_START(rra_start) + _page_size;
	for (i = 0; i < rrd->stat_head->rra_cnt; ++i) {
		active_block =
			PAGE_START(rra_start
					+ rrd->rra_ptr[i].cur_row
					* rrd->stat_head->ds_cnt * sizeof(rrd_value_t));
		if (active_block > dontneed_start) {
#ifdef USE_MADVISE
			madvise(rrd_simple_file->file_start + dontneed_start,
					active_block - dontneed_start - 1, MADV_DONTNEED);
#endif
			/* in linux at least only fadvise DONTNEED seems to purge pages from cache */
#ifdef HAVE_POSIX_FADVISE
			posix_fadvise(rrd_simple_file->fd, dontneed_start,
					active_block - dontneed_start - 1,
					POSIX_FADV_DONTNEED);
#endif
		}
		dontneed_start = active_block;
		/* do not release 'hot' block if update for this RAA will occur
		 * within 10 minutes */
		if (rrd->stat_head->pdp_step * rrd->rra_def[i].pdp_cnt -
				rrd->live_head->last_up % (rrd->stat_head->pdp_step *
					rrd->rra_def[i].pdp_cnt) < 10 * 60) {
			dontneed_start += _page_size;
		}
		rra_start +=
			rrd->rra_def[i].row_cnt * rrd->stat_head->ds_cnt *
			sizeof(rrd_value_t);
	}

	if (dontneed_start < rrd_file->file_len) {
#ifdef USE_MADVISE
		madvise(rrd_simple_file->file_start + dontneed_start,
				rrd_file->file_len - dontneed_start, MADV_DONTNEED);
#endif
#ifdef HAVE_POSIX_FADVISE
		posix_fadvise(rrd_simple_file->fd, dontneed_start,
				rrd_file->file_len - dontneed_start,
				POSIX_FADV_DONTNEED);
#endif
	}

#if defined DEBUG && DEBUG > 1
	mincore_print(rrd_file, "after");
#endif
#endif                          /* without madvise and posix_fadvise it does not make much sense todo anything */
}





int rrd_close(
		rrd_file_t *rrd_file)
{
	rrd_simple_file_t *rrd_simple_file;
	rrd_simple_file = (rrd_simple_file_t *)rrd_file->pvt;
	int       ret;

#ifdef HAVE_MMAP
	ret = msync(rrd_simple_file->file_start, rrd_file->file_len, MS_ASYNC);
	if (ret != 0){
		ret = -RRD_ERR_MSYNC;
		goto out;
	}
	ret = munmap(rrd_simple_file->file_start, rrd_file->file_len);
	if (ret != 0){
		ret = -RRD_ERR_MUNMAP;
		goto out;
	}
#endif
	ret = close(rrd_simple_file->fd);
	if (ret != 0){
		ret = -RRD_ERR_CLOSE;
		goto out;
	}
out:
	free(rrd_file->pvt);
	free(rrd_file);
	rrd_file = NULL;
	return ret;
}


/* Set position of rrd_file.  */

off_t rrd_seek( rrd_file_t *rrd_file, off_t off, int whence) {
	off_t     ret = 0;
#ifndef HAVE_MMAP
	rrd_simple_file_t *rrd_simple_file;
	rrd_simple_file = (rrd_simple_file_t *)rrd_file->pvt;
#endif

#ifdef HAVE_MMAP
	if (whence == SEEK_SET)
		rrd_file->pos = off;
	else if (whence == SEEK_CUR)
		rrd_file->pos += off;
	else if (whence == SEEK_END)
		rrd_file->pos = rrd_file->file_len + off;
#else
	ret = lseek(rrd_simple_file->fd, off, whence);
	rrd_file->pos = ret;
#endif
	/* mimic fseek, which returns 0 upon success */
	return ret < 0;     /*XXX: or just ret to mimic lseek */
}


/* Get current position in rrd_file.  */

off_t rrd_tell(
		rrd_file_t *rrd_file)
{
	return rrd_file->pos;
}


/* Read count bytes into buffer buf, starting at rrd_file->pos.
 * Returns the number of bytes read or <0 on error.  */

ssize_t rrd_read(
		rrd_file_t *rrd_file,
		void *buf,
		size_t count)
{
	rrd_simple_file_t *rrd_simple_file = (rrd_simple_file_t *)rrd_file->pvt;
#ifdef HAVE_MMAP
	size_t    _cnt = count;
	ssize_t   _surplus;

	if (rrd_file->pos > rrd_file->file_len || _cnt == 0)    /* EOF */
		return 0;
	if (buf == NULL)
		return -1;      /* EINVAL */
	_surplus = rrd_file->pos + _cnt - rrd_file->file_len;
	if (_surplus > 0) { /* short read */
		_cnt -= _surplus;
	}
	if (_cnt == 0)
		return 0;       /* EOF */
	buf = memcpy(buf, rrd_simple_file->file_start + rrd_file->pos, _cnt);

	rrd_file->pos += _cnt;  /* mimmic read() semantics */
	return _cnt;
#else
	ssize_t   ret;

	ret = read(rrd_simple_file->fd, buf, count);
	if (ret > 0)
		rrd_file->pos += ret;   /* mimmic read() semantics */
	return ret;
#endif
}


/* Write count bytes from buffer buf to the current position
 * rrd_file->pos of rrd_simple_file->fd.
 * Returns the number of bytes written or <0 on error.  */

ssize_t rrd_write(rrd_file_t *rrd_file, const void *buf, size_t count){
	rrd_simple_file_t *rrd_simple_file = (rrd_simple_file_t *)rrd_file->pvt;
#ifdef HAVE_MMAP
	size_t old_size = rrd_file->file_len;
	if (count == 0)
		return 0;
	if (buf == NULL)
		return -1;      /* EINVAL */

	if((rrd_file->pos + count) > old_size) {
		return -RRD_ERR_WRITE6;
	}
	memcpy(rrd_simple_file->file_start + rrd_file->pos, buf, count);
	rrd_file->pos += count;
	return count;       /* mimmic write() semantics */
#else
	ssize_t   _sz = write(rrd_simple_file->fd, buf, count);

	if (_sz > 0)
		rrd_file->pos += _sz;
	return _sz;
#endif
}


/* this is a leftover from the old days, it serves no purpose
   and is therefore turned into a no-op */
void rrd_flush(rrd_file_t UNUSED(*rrd_file))
{
}

/* Initialize RRD header.  */

void rrd_init(rrd_t *rrd) {
	rrd->stat_head = NULL;
	rrd->ds_def = NULL;
	rrd->rra_def = NULL;
	rrd->live_head = NULL;
	rrd->legacy_last_up = NULL;
	rrd->rra_ptr = NULL;
	rrd->pdp_prep = NULL;
	rrd->cdp_prep = NULL;
	rrd->rrd_value = NULL;
}


/* free RRD header data.  */

#ifdef HAVE_MMAP
void rrd_free(rrd_t *rrd) {
	if (rrd->legacy_last_up) {  /* this gets set for version < 3 only */
		free(rrd->live_head);
	}
}
#else
void rrd_free(rrd_t *rrd) {
	free(rrd->live_head);
	free(rrd->stat_head);
	free(rrd->ds_def);
	free(rrd->rra_def);
	free(rrd->rra_ptr);
	free(rrd->pdp_prep);
	free(rrd->cdp_prep);
	free(rrd->rrd_value);
}
#endif


/* routine used by external libraries to free memory allocated by
 * rrd library */

void rrd_freemem(void *mem) {
	free(mem);
}

/*
 * rra_update informs us about the RRAs being updated
 * The low level storage API may use this information for
 * aligning RRAs within stripes, or other performance enhancements
 */
void rrd_notify_row(rrd_file_t UNUSED(*rrd_file),
			int UNUSED(rra_idx), unsigned long UNUSED(rra_row),
			time_t UNUSED(rra_time)) {
}

/*
 * This function is called when creating a new RRD
 * The storage implementation can use this opportunity to select
 * a sensible starting row within the file.
 * The default implementation is random, to ensure that all RRAs
 * don't change to a new disk block at the same time
 */
unsigned long rrd_select_initial_row( rrd_file_t UNUSED(*rrd_file),
			int UNUSED(rra_idx), rra_def_t *rra) {
	return rrd_random() % rra->row_cnt;
}
