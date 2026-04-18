#!/bin/bash
set -e
echo "[1/4] Pushing newly built binary and matching libc_helper.so to printer via rsync..."
sshpass -p rockchip rsync -av --progress gklib_uclibc_current root@192.168.0.96:/userdata/app/gk/gklib_candidate
sshpass -p rockchip rsync -av --progress internal/pkg/chelper/libc_helper.so root@192.168.0.96:/userdata/app/gk/libc_helper.so

LOCAL_MD5=$(md5sum gklib_uclibc_current | awk '{print $1}')
echo "Local binary MD5: $LOCAL_MD5"

echo "[2/4] Verifying remote binary MD5 checksum..."
REMOTE_MD5=$(sshpass -p rockchip ssh -o StrictHostKeyChecking=no root@192.168.0.96 "md5sum /userdata/app/gk/gklib_candidate | awk '{print \$1}'")
echo "Remote binary MD5: $REMOTE_MD5"

if [ "$LOCAL_MD5" != "$REMOTE_MD5" ]; then
  echo "ERROR: MD5 checksums do not match!"
  exit 1
fi
echo "MD5 checksums match - transfer verified."

echo "[3/4] Stopping services and installing binary..."
sshpass -p rockchip ssh -o StrictHostKeyChecking=no root@192.168.0.96 << 'SSH_EOF'
killall -9 gklib 2>/dev/null || true

cd /userdata/app/gk
# Leave pure stock backup if not present, replace the binary
cp gklib_candidate gklib
chmod 755 gklib

# Flush filesystem buffers to disk
sync
sync
sync
SSH_EOF

echo "[4/4] Please reboot the printer to test the new binary."
