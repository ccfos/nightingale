#!/bin/sh
FILE=rrd_error.h
echo '#define RRD_ERR_START                            0x0200' > $FILE
cat ./rrd_error.c | sed 's|.*\/\*[ ]*\([^\s]*\)[ ]*\*\/.*|\1|' | grep '^RRD_ERR' | awk '{printf("#define %-40s 0x%04x\n", $1, NR+0x1ff)}' >> $FILE
wc -l $FILE | awk '{printf("/* if add new system event flag, please upadte the RRD_ERR_END */\n#define RRD_ERR_END                              0x%04x\n#define RRD_ERR_NUM                              (RRD_ERR_END - RRD_ERR_START + 1)", $1 - 1 + 0x1ff)}' >> $FILE


