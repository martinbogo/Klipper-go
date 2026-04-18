#!/bin/sh
echo "Stopping services..."
killall -9 gklib
killall -9 K3SysUi
sleep 2

echo "Restoring factory copies..."
cp -f /userdata/app/gk/gklib.factory /userdata/app/gk/gklib
cp -f /userdata/app/gk/libc_helper.so.factory /userdata/app/gk/libc_helper.so
cp -f /useremain/rinkhals/.current/start.sh.factory /useremain/rinkhals/.current/start.sh
chmod 755 /userdata/app/gk/gklib
chmod 755 /userdata/app/gk/libc_helper.so
chmod 755 /useremain/rinkhals/.current/start.sh

echo "Restarting application service..."
/etc/init.d/S99app.sh restart
echo "Factory state restored."
