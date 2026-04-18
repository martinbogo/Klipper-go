#!/bin/sh
echo "Stopping running services..."
killall -9 gklib gkapi K3SysUi 2>/dev/null || true
sleep 1

echo "Restoring factory binaries from backup..."
cd /userdata/app/gk
if [ -f gklib.factory.bak ]; then cp -f gklib.factory.bak gklib; fi
if [ -f libc_helper.so.factory.bak ]; then cp -f libc_helper.so.factory.bak libc_helper.so; fi
if [ -f start.sh.factory.bak ]; then cp -f start.sh.factory.bak start.sh; fi

chmod 755 gklib libc_helper.so start.sh

# The factory firmware runs Klipper-go from /userdata/app/kenv/run.sh invoked by /etc/init.d/S90_app_run
echo "Restarting application service..."
/etc/init.d/S90_app_run start
sleep 4
echo "Factory state restored! Tailing /tmp/gklib.log..."
tail -n 10 /tmp/gklib.log 2>/dev/null
