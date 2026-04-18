# KS1 and KS1 Max MCU reset

The Kobra S1 and Kobra S1 Max expose an MCU reset line on Linux sysfs GPIO `116`.

This corresponds to asserting `gpio16` on the printer host. During testing, pulsing that line high for one second forces an MCU reboot.

## Tool

The repository includes a Linux-only helper at `cmd/mcu_reset`.

Defaults:

- printer profile: `ks1`
- sysfs GPIO: `116`
- pre-delay after export/setup: `1s`
- asserted high duration: `1s`
- deassert back to `0` after the pulse

## Example usage

Run locally on the printer host:

```text
go run ./cmd/mcu_reset --printer ks1
```

For the Max profile:

```text
go run ./cmd/mcu_reset --printer ks1m
```

Optional flags:

- `--gpio` to override the sysfs GPIO number
- `--pre-delay` to change the setup delay before assertion
- `--hold` to change the asserted-high duration
- `--cleanup` to unexport the GPIO after the pulse
- `--sysfs-root` for tests or chroot-style environments

## Equivalent shell sequence

```text
echo 116 > /sys/class/gpio/export
echo out > /sys/class/gpio/gpio116/direction
sleep 1
echo 1 > /sys/class/gpio/gpio116/value
sleep 1
echo 0 > /sys/class/gpio/gpio116/value
```
