#!/bin/sh
# Test gklib_real directly (no shim) to verify MCU communication
set -e
killall gklib gklib_real serial_shim 2>/dev/null || true
sleep 1
/userdata/app/gk/mcu_reset --printer ks1 --pre-delay 1s --hold 1s --cleanup
sleep 5
cd /userdata/app/gk
export LD_LIBRARY_PATH=/userdata/app/gk:$LD_LIBRARY_PATH
./gklib_real -a /tmp/unix_uds_test_direct rinkhals_gklib.cfg >/tmp/gklib_real_direct.out 2>&1 &
GPID=$!
sleep 18
kill $GPID 2>/dev/null || true
sleep 1
kill -9 $GPID 2>/dev/null || true
echo "=== RESULT ==="
grep -c "project state: Ready" /tmp/gklib_real_direct.out || echo "0"
grep -E "project state:|Timeout|TMC|tmcuart|IFCNT" /tmp/gklib_real_direct.out | tail -20
