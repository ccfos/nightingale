/*****************************************************************************
 * RRDtool 1.4.9  Copyright by Tobi Oetiker, 1997-2014
 * This file:     Copyright 2003 Peter Stamfest <peter@stamfest.at> 
 *                             & Tobias Oetiker
 * Distributed under the GPL
 *****************************************************************************
 * rrd_is_thread_safe.c   Poisons some nasty function calls using GNU cpp
 *****************************************************************************
 * $Id$
 *************************************************************************** */

#ifndef _RRD_IS_THREAD_SAFE_H
#define _RRD_IS_THREAD_SAFE_H

#ifdef  __cplusplus
extern    "C" {
#endif

#undef strerror

#if( 2 < __GNUC__ )
#pragma GCC poison strtok asctime ctime gmtime localtime tmpnam strerror
#endif

#ifdef  __cplusplus
}
#endif
#endif /*_RRD_IS_THREAD_SAFE_H */
