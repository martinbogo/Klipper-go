# K3C Refactored Architecture Plan

Based on the symbol extraction from the remote binary (`gklib_remote`), the target application has been significantly refactored into a structured, domain-driven package hierarchy under `k3c/internal/...`. Here is the proposed local repository layout to match the refactored binary.

## Proposed Directory Structure

```
k3c-go/
├── cmd/
│   ├── gklib/               # Main Application entrypoint (previously main/K3cMain.go)
│   └── config_convert/      # Utility Binary entrypoint
├── internal/
│   ├── addon/               # Add-on packages and proxy modules
│   ├── print/               # Printing state and execution logic
│   └── pkg/                 # Domain-driven core Klipper functionality
│       ├── chelper/         # C-bindings and iter-solve helpers
│       ├── fan/             # Fan control (controller, variable speeds)
│       ├── filament/        # Filament runout, motion, and switch sensors
│       │   └── ace_v2/      # ACE V2 specific hardware/spool management protocols
│       ├── gcode/           # G-code parser, macros, and evaluation
│       ├── heater/          # Heaters, generic heaters, temperature sensors
│       ├── mcu/             # MCU communication, ADCs, bus, limits
│       ├── motion/          # Kinematics, trajectory generation, trapq
│       │   └── vibration/   # ADXL345, resonance testing, input shaper
│       ├── msgproto/        # Network message protobuf wrappers and serialization
│       ├── printer/         # Overall printer config parsing and board setup
│       ├── serialhdl/       # Lower-level serial queues and interactions
│       ├── toolhead/        # High-level toolhead kinematics wrapper
│       └── web/             # Network API endpoints and handlers
├── go.mod                   # Updated to go 1.23, module defined as `k3c`
└── Makefile                 # Updated targets using the new paths
```

## Migration Steps

1. **Go Toolchain Upgrade:** Upgrade `go.mod` to Go 1.23 and rename the module to `k3c`.
2. **Move Shared/Helper Code:** Move `chelper` into `internal/pkg/chelper`.
3. **Partition Existing Features:** Break apart the flat `project/` directory into domain packages. For example:
   *   `extras_extruder_stepper.go` -> `internal/pkg/toolhead/`
   *   `extras_heater_bed.go` -> `internal/pkg/heater/`
   *   `extras_fan.go` -> `internal/pkg/fan/`
   *   `extras_adxl345.go` -> `internal/pkg/motion/vibration/`
4. **Scaffolding Stubs:** Create stubs for net-new systems seen in the binary:
   *   `internal/pkg/filament/ace_v2`
   *   `internal/addon`
   *   `internal/print`
5. **Update Entrypoint:** Rewrite the main application lifecycle in `cmd/gklib/main.go` to instantiate these domain managers appropriately.

## Current Migration Status (2026-04-05)

The repository is materially farther along than the original draft above. The `internal/` layout is no longer just scaffolding: `internal/pkg/gcode`, `heater`, `io`, `mcu`, `motion/probe`, `motion/vibration`, `printer`, `serialhdl`, `util`, `internal/addon`, and `internal/print` are all active and wired into the running system.

Recent completed slices include:

- removal of the thin `serialhdl` compatibility layer from `project/`
- removal of the duplicate `ModuleConfig` wrapper around `ConfigWrapper`
- removal of the `query_adc` bridge/adapter layer from `project/`
- direct source compilation for `internal/pkg/chelper` on `linux/arm64`
- migration of cartesian/corexy/none kinematics into `internal/pkg/motion/kinematics`
- migration of the homing engine into `internal/pkg/motion/homing` with project-facing compatibility wrappers
- migration of `PrinterHoming` G28/manual/probe command orchestration into `internal/pkg/motion/homing`, while keeping the constructor/runtime lookup shell in `project/extras_homing.go`
- migration of the manual-probe helper/session engine into `internal/pkg/motion/probe`, while keeping the `ManualProbe` module and loader in `project/`
- migration of `PrinterProbe` command and calibration handlers into `internal/pkg/motion/probe`, while keeping the constructor/config shell in `project/extras_probe.go`
- migration of the remaining `ManualProbe` command handlers into `internal/pkg/motion/probe`, while keeping the constructor/config parsing shell in `project/extras_manual_probe.go`
- migration of `ProbePointsHelper` probing/manual handoff sequencing into `internal/pkg/motion/probe`, while keeping its constructor/config parsing shell in `project/extras_probe.go`
- migration of the `PrinterProbe.Run_probe` sampling/retry loop into `internal/pkg/motion/probe`, while keeping the probe motion/homing shell in `project/extras_probe.go`
- migration of `ProbeEndstopWrapper` runtime orchestration into `internal/pkg/motion/probe`, while keeping its config/template and raw MCU pin-setup shell in `project/extras_probe.go`
- migration of the `PrinterProbe.Probe` single-probe motion path into `internal/pkg/motion/probe`, while keeping the homing/runtime adapter shell in `project/extras_probe.go`
- migration of `PrinterProbe` homing-event and multi-probe orchestration into `internal/pkg/motion/probe`, while keeping the constructor/config parsing and endstop-adapter shell in `project/extras_probe.go`
- migration of the remaining `ProbeEndstopWrapper` MCU-identify stepper hookup and probe virtual-endstop validation into `internal/pkg/motion/probe`, while keeping the config/template and raw MCU pin-setup shell in `project/extras_probe.go`
- consolidation of the thin project-side probe adapter layer into `project/probe_adapters.go`, deleting the old per-adapter `project/probe_command_adapters.go`, `project/probe_endstop_adapters.go`, `project/probe_event_adapters.go`, `project/probe_motion_adapters.go`, `project/probe_points_adapters.go`, and `project/probe_run_adapters.go` files while preserving the same package-private bridge types
- consolidation of the thin project-side manual-probe and motion-report adapter helpers into `project/manual_probe_adapters.go` and `project/motion_report_adapters.go`, deleting the old `project/manual_probe_command_adapters.go` and `project/api_dump_adapters.go` files while preserving the same package-private bridge types
- migration of the bed-mesh `ZMesh` interpolation engine and `MoveSplitter` move-segmentation core into `internal/pkg/motion/bed_mesh`, while keeping the module/config/profile/calibration shell in `project/extras_bed_mesh.go`
- migration of bed-mesh calibration point generation and algorithm normalization into `internal/pkg/motion/bed_mesh`, while keeping config parsing, adaptive-mesh policy, probing orchestration, and profile persistence in `project/extras_bed_mesh.go`
- migration of motion-report stepper and trapq dump processing into `internal/pkg/motion/report`, while keeping API timer/webhook plumbing and printer-runtime registration in `project/extras_motion_report.go`
- migration of motion-report tracked-name sorting, shutdown window planning, and live status cache updates into `internal/pkg/motion/report`, while keeping printer lookups, trapq/stepper adapters, and shutdown logging in `project/extras_motion_report.go`
- migration of the shared API dump helper and internal dump client into `internal/pkg/motion/report`, while keeping reactor/web-request adapters and module-specific start/stop callbacks in `project/`
- migration of the shared accelerometer query helper, G-code command helper, and chip clock-sync regression into `internal/pkg/motion/vibration/accelerometer_helpers.go`, while keeping `project/extras_adxl345.go` and `project/extras_lis2dw12.go` as the concrete SPI/bulk-query shells and deleting `project/extras_accelerometer.go`
- migration of the G-code macro runtime (rename handling, variable mutation, recursion guard, and script execution context assembly) into `internal/pkg/gcode/macro_module.go`, while keeping `project/extras_gcode_macro.go` as the Jinja/template host shell for `PrinterGCodeMacro` and `TemplateWrapper`
- migration of the toolhead drip-mode advance loop and drip-end sentinel into `internal/pkg/motion/toolhead_drip.go`, while keeping the remaining project toolhead shell as the concrete reactor/completion and trapq owner
- migration of the toolhead pause-check application plus the priming/flush timer callback runtime into `internal/pkg/motion/toolhead_timer_runtime.go`, while keeping the remaining project toolhead shell as the concrete reactor timer and lookahead executor
- migration of the toolhead wait wrapper and drip-move entry/error/cleanup orchestration into `internal/pkg/motion/toolhead_move_runtime.go`, while keeping the remaining project toolhead shell as the concrete lookahead queue and trapq finalizer
- migration of toolhead move/manual-move/dwell/velocity-limit control helpers into `internal/pkg/motion/toolhead_control_runtime.go`, and deletion of `project/toolhead.go` in favor of smaller `project/toolhead_*.go` shell files
- migration of the motion-report `DumpStepper` wrapper into `internal/pkg/motion/report`, while keeping endpoint registration, raw `MCU_stepper` ownership, and shutdown-time MCU lookup in `project/extras_motion_report.go`
- migration of the motion-report `DumpTrapQ` wrapper into `internal/pkg/motion/report`, while keeping endpoint registration, raw trapq ownership, and printer-runtime lookup in `project/extras_motion_report.go`
- migration of the core `Move` and `LookAheadQueue` motion planning logic into `internal/pkg/motion`, while keeping the remaining project toolhead shell as the printer-runtime scheduling and trapq owner
- migration of extruder move validation, junction calculation, and trapq move planning into `internal/pkg/motion`, while keeping `project/kinematics_extruder.go` as the heater/config/trapq shell
- migration of the reusable legacy extruder runtime/status shell and dummy extruder behavior into `internal/pkg/motion`, while keeping heater setup, extruder-stepper ownership, and G-code command wiring in `project/kinematics_extruder.go`
- migration of toolhead extruder-queue dispatch onto the shared internal extruder contract, removing the remaining `*PrinterExtruder` type assertion from `project/toolhead_runtime_adapters.go`
- migration of toolhead initial velocity-limit state shaping and junction-deviation planning into `internal/pkg/motion/toolhead_config.go`, while keeping config reads and module bootstrap in `project/toolhead_module.go`
- migration of toolhead G4/M400/SET_VELOCITY_LIMIT/M204 command handling into `internal/pkg/motion/toolhead_commands.go`, while keeping G-code registration and project-side printer/state adapters in `project/toolhead_commands.go`
- migration of scaled-ADC chip initialization/orchestration into `internal/pkg/heater/adc_scaled_module.go`, while keeping `project/extras_adc_scaled.go` as the concrete MCU-pin and virtual-chip registration shell
- migration of shared accelerometer FIFO/status clock-update sequencing into `internal/pkg/motion/vibration/chip_clock_sync.go`, while keeping `project/extras_adxl345.go` and `project/extras_lis2dw12.go` as the concrete bulk-query/status shells
- migration of the core move-batch scheduling loop into `internal/pkg/motion`, while keeping the remaining project toolhead shell as the print-time/drip/flush shell and trapq sink adapter
- migration of the toolhead print-time advancement, flush scheduling, step-generation scan-window tracking, and movequeue activity bookkeeping into `internal/pkg/motion`, while keeping timer callbacks, pause/priming state, and drip-mode orchestration in the remaining `project/toolhead_*.go` shell
- migration of toolhead status, busy-state, and stats payload shaping into `internal/pkg/motion`, while keeping MCU activity checks, kinematics/extruder lookups, and queue ownership in the remaining `project/toolhead_*.go` shell
- migration of toolhead pause throttling, priming transition decisions, and wait-until-idle loop control into `internal/pkg/motion`, while keeping reactor timer ownership and concrete pause scheduling in the remaining `project/toolhead_*.go` shell
- migration of toolhead lookahead-reset state shaping and flush-handler scheduling decisions into `internal/pkg/motion`, while keeping lookahead flush execution, flush-time advancement, and timer callbacks in the remaining `project/toolhead_*.go` shell
- migration of MCU query-slot calculation, steppersync flush/timeout checks, and stepper active/generate timing helpers into `internal/pkg/mcu`, while keeping MCU lifecycle and the concrete chelper/serial objects in `project/mcu_core.go`, `project/mcu_connection.go`, `project/mcu_runtime.go`, and `project/mcu_lifecycle.go`, plus the remaining project-side stepper shell in `project/mcu_stepper.go` and `project/printer_rail.go`
- migration of `MCU_stepper` position math and stepcompress reset/last-position sync helpers into `internal/pkg/mcu`, while keeping stepper construction, itersolve binding, event registration, and the raw query/chelper shells in `project/mcu_stepper.go`
- migration of `MCU_stepper` step-pulse defaulting and both-edge config planning into `internal/pkg/mcu`, while keeping stepper config command emission, command lookup, and `stepcompress` setup in `project/mcu_stepper.go`
- migration of `MCU_stepper` config-command strings, lookup formats, and stepcompress setup metadata into `internal/pkg/mcu`, while keeping concrete MCU command registration, query-wrapper binding, and `stepcompress` allocation in `project/mcu_stepper.go`
- migration of `PrinterRail` explicit endstop-pin normalization, lookup, and shared-settings conflict planning into `internal/pkg/mcu`, while keeping pin parsing, concrete endstop creation, and printer registration in `project/printer_rail.go`
- migration of `PrinterRail` position-range validation and homing-direction inference into `internal/pkg/motion/kinematics`, while keeping config reads, endstop discovery, and concrete printer wiring in `project/printer_rail.go`
- migration of `PrinterStepper` config-driven pin lookup, step-distance / pulse-duration planning, and helper-module registration sequencing into `internal/pkg/mcu`, while keeping concrete `MCU_stepper` construction and module-specific type assertions in `project/mcu_stepper.go`
- migration of step-distance, units-in-radians inference, and gear-ratio parsing into `internal/pkg/motion/kinematics`, while keeping `MCU_stepper` construction and project-side module registration in `project/mcu_stepper.go` and `project/extras_tmc.go`
- deletion of `project/stepper.go` in favor of focused project-side stepper shells, now `project/mcu_stepper.go` and `project/printer_rail.go`, while preserving the remaining `MCU_stepper` / `PrinterRail` shell in smaller files
- migration of `MCU_trsync` runtime-state handling and start-plan calculation into `internal/pkg/mcu`, while keeping trsync config/query wrappers in `project/mcu_trsync.go` and the broader MCU lifecycle shell in `project/mcu_core.go`, `project/mcu_connection.go`, `project/mcu_runtime.go`, and `project/mcu_lifecycle.go`
- migration of `MCU_endstop` homing runtime planning, wait-result resolution, and query-state helpers into `internal/pkg/mcu`, while keeping endstop config/build, trsync collection/stepper registration, and raw command/chelper shells in `project/mcu_endstop.go`
- migration of `MCU_adc` runtime value scaling and read-time calculation into `internal/pkg/mcu`, while keeping ADC config/build, pin registration, and callback registration in `project/mcu_adc.go`
- migration of `MCU_adc` config-planning math into `internal/pkg/mcu`, while keeping ADC config command emission and response registration in `project/mcu_adc.go`
- migration of `MCU_digital_out` and `MCU_pwm` runtime value normalization, inversion, and queued-send helpers into `internal/pkg/mcu`, while keeping output-pin config/build and command lookup shells in `project/mcu_digital_out.go` and `project/mcu_pwm.go`
- migration of `MCU_digital_out` and `MCU_pwm` config-planning math into `internal/pkg/mcu`, while keeping output-pin config command emission and command lookup shells in `project/mcu_digital_out.go` and `project/mcu_pwm.go`
- migration of MCU SPI/I2C config-command strings and lookup formats into `internal/pkg/mcu`, while keeping bus-name resolution, concrete MCU command registration, and transfer/runtime wrappers in `project/extras_bus.go`
- migration of MCU SPI/I2C deferred command formatting (shutdown/send/write/modify-bits config payloads) into `internal/pkg/mcu`, while keeping bus-name resolution and concrete command-wrapper sends in `project/extras_bus.go`
- migration of MCU SPI/I2C bus-name resolution and live SPI/I2C session/runtime helpers into `internal/pkg/mcu`, while keeping `project/extras_bus.go` as the config-based constructor shell that owns OID/command-queue allocation and adapts raw `*MCU` / pin-resolver APIs into those internal runtimes
- migration of shared TMC register-field packing/formatting and shared wave-table/microstep/stealthchop helper math into `internal/pkg/tmc`, while keeping TMC command registration, printer-event handling, and sensorless-homing/runtime shells in `project/extras_tmc.go`
- rewiring of the core TMC driver constructors (`2130`, `2208`, `2209`, `2240`, `2660`, `5160`) to instantiate their shared field/register helpers from `internal/pkg/tmc` instead of the old project-local implementation
- migration of the static TMC driver register maps, field tables, field formatters, and constructor-side default setup for `2130`, `2208`, `2209`, `2240`, `2660`, and `5160` into `internal/pkg/tmc`, while keeping the project-side transport selection, current helpers, and concrete MCU ownership shells in `project/extras_tmc*.go`
- migration of the shared `TMCCommandHelper` and `TMCVirtualPinHelper` state-machine core into `internal/pkg/tmc`, while keeping `project/extras_tmc.go` as the printer-event, G-code, homing-endstop, and runtime adapter shell
- migration of the shared TMC current-control helpers for `2208`/`2209`/`2130`, `2240`, and `5160` into `internal/pkg/tmc`, while keeping the SPI/UART transport wrappers, the `2660` idle-current event helper, and the concrete MCU ownership/runtime shells in `project/extras_tmc*.go`
- migration of the shared TMC UART CRC/bit-packing/read-decode logic and SPI daisy-chain frame packing/response decode helpers into `internal/pkg/tmc`, while keeping MCU pin/config lookup, command-queue registration, and concrete SPI/UART transport ownership in `project/extras_tmc2130.go` and `project/extras_tmc_uart.go`
- migration of the `TMC2660` current helper, including idle-current event handling, into `internal/pkg/tmc`, while keeping `project/extras_tmc2660.go` as the raw SPI transport and constructor shell
- removal of the legacy `project/extras_tmc*.go` file set in favor of neutral `project/tmc_*.go` files, so the remaining project-side TMC code is no longer anchored to the old extras naming even though the deeper `project/` package deprecation is still in progress
- migration of the per-driver TMC module assembly/load path into `internal/pkg/tmc/driver_modules.go`, behind a shared `tmc.DriverAdapter` seam; `project/driver_adapters_tmc.go` now provides the compatibility constructors/loaders plus the remaining printer-event, G-code, virtual-pin, and config-driven setup wiring, while the remaining raw UART/SPI transport ownership shell is split across `project/tmc_uart_*.go` and `project/tmc_spi_*.go`. The old per-driver `project/tmc2130.go`, `project/tmc2208.go`, `project/tmc2209.go`, `project/tmc2240.go`, `project/tmc2660.go`, and `project/tmc5160.go` files have been removed.
- migration of the `TMCErrorCheck` periodic register/temperature monitoring runtime into `internal/pkg/tmc/error_check_runtime.go`, with the remaining concrete reactor/shutdown/heater-monitor hookup now living in `project/driver_adapters_tmc.go`
- migration of the `TMCCommandHelper` runtime/state bookkeeping into `internal/pkg/tmc/command_runtime.go`, with the remaining command registration, G-code argument parsing/response formatting, reactor callback scheduling, and raw `*MCU_stepper` event-payload adaptation now living in `project/driver_adapters_tmc.go`
- migration of the `TMCVirtualPinHelper` homing runtime/orchestration into `internal/pkg/tmc/virtual_pin_runtime.go`, with the remaining pin setup, event registration, and raw homing-move endstop payload adaptation now living in `project/driver_adapters_tmc.go`
- migration of the TMC UART named-register retry and IFCNT-confirmation flow into `internal/pkg/tmc/uart_transport_runtime.go`, while the remaining project-side UART transport shell in `project/tmc_uart_*.go` keeps the raw MCU bitbang transport, mux activation, config callbacks, and the thin register-access wrapper around that internal runtime
- migration of TMC UART shared mutex caching and mux-instance coordination into `internal/pkg/tmc/uart_resource_runtime.go`, while the remaining project-side UART transport shell in `project/tmc_uart_*.go` keeps MCU pin lookup, mux config/build callbacks, and concrete bitbang transport ownership
- migration of the shared SPI daisy-chain position/transfer runtime and TMC2660 read-select runtime into `internal/pkg/tmc/spi_transport_runtime.go`, while the remaining project-side SPI transport shell in `project/tmc_spi_*.go` keeps raw `MCU_SPI` ownership, config lookup, and the thin register-access wrappers around those internal runtimes
- deletion of `project/spi_tmc_transport.go` and `project/tmc_transport_uart.go` in favor of focused `project/tmc_spi_*.go` and `project/tmc_uart_*.go` files, while preserving the public TMC transport constructors and register-access APIs
- migration of `MCU_endstop` trsync selection, shared-axis conflict detection, and stepper flattening into `internal/pkg/mcu`, while keeping new trsync allocation, endstop config/build, and raw command/chelper shells in `project/mcu_endstop.go`
- split of the remaining `MCU_trsync` and `MCU_endstop` project-side shells out of the legacy `project/Mcu.go` into focused `project/mcu_trsync.go` and `project/mcu_endstop.go` files, leaving the project-side MCU shell centered on broader MCU lifecycle/bootstrap work
- migration of MCU bootstrap config-command compilation/CRC planning and connection-mode selection into `internal/pkg/mcu`, while keeping config callback execution, serial I/O, and printer lifecycle ownership in the project-side MCU shell
- migration of MCU stats accumulation, ready-frequency validation, and stats-line formatting into `internal/pkg/mcu`, while keeping serial/clocksync plumbing and status ownership in the project-side MCU shell
- migration of MCU config-session parsing, clear-shutdown decision logic, and connect-state planning into `internal/pkg/mcu`, while keeping query-command transport, config transmission, and steppersync allocation in the project-side MCU shell
- migration of MCU identify-finalization planning (reserved pins, restart-method fallback, bridge detection, and status-info assembly) into `internal/pkg/mcu`, while keeping parser access, command lookup, and event registration in the project-side MCU shell
- migration of MCU shutdown parsing, spontaneous-restart detection, and automated-restart decision logic into `internal/pkg/mcu`, while keeping printer shutdown side effects, serial/clocksync ownership, and event registration in the project-side MCU shell
- migration of MCU command-reset planning and firmware-restart action selection into `internal/pkg/mcu`, while keeping concrete reset command sends, disconnects, and transport-specific restart implementations in the project-side MCU shell
- migration of MCU emergency-stop gating and MCU info/rollover formatting into `internal/pkg/mcu`, while keeping emergency-stop sends, message parser access, and printer rollover updates in the project-side MCU shell
- migration of MCU trsync and endstop config-command planning into `internal/pkg/mcu`, while keeping command lookup, query wrapper binding, and `chelper` object ownership in `project/mcu_trsync.go` and `project/mcu_endstop.go`
- migration of MCU ADC config-command setup metadata into `internal/pkg/mcu`, while keeping OID allocation and response registration in the project-side MCU pin shell
- migration of MCU digital-out and PWM config-command setup metadata into `internal/pkg/mcu`, while keeping move-queue reservation, OID allocation, and command-wrapper binding in the project-side MCU pin shell
- split of the remaining `MCU_digital_out`, `MCU_pwm`, and `MCU_adc` project-side shells out of the legacy `project/Mcu.go` into focused `project/mcu_digital_out.go`, `project/mcu_pwm.go`, and `project/mcu_adc.go` files, leaving the project-side MCU shell centered more tightly on MCU lifecycle/bootstrap work
- split of the remaining project-side MCU connection/runtime/lifecycle methods out of the legacy `project/Mcu.go` into focused `project/mcu_connection.go`, `project/mcu_runtime.go`, and `project/mcu_lifecycle.go` files, leaving the project-side MCU shell centered on the core `MCU` type and constructor
- deletion of the legacy `project/Mcu.go` file in favor of `project/mcu_core.go`, with the project-side MCU shell now living across `project/mcu_core.go`, `project/mcu_connection.go`, `project/mcu_runtime.go`, `project/mcu_lifecycle.go`, and the dedicated `project/mcu_*.go` hardware shells
- integration of the LeviQ3 core workflow into `internal/print/leviQ3.go`, with `project/extras_leviq3.go` reduced to the printer-runtime, persistence, and G-code adapter shell
- LeviQ3 persistence now round-trips recovered mesh and applied-offset state through the internal `print` boundary instead of silently dropping that runtime state in the `project/` shell

That means the remaining work is now mostly concentrated in the hardest parts of the old flat architecture: the remaining project-side `Toolhead` queue/trapq/bootstrap shell (`project/toolhead_*.go`), the remaining project-side stepper shell (`project/mcu_stepper.go` and `project/printer_rail.go`), the project-side MCU shell (`project/mcu_core.go`, `project/mcu_connection.go`, `project/mcu_runtime.go`, and `project/mcu_lifecycle.go`), the dedicated `project/mcu_trsync.go`, `project/mcu_endstop.go`, `project/mcu_digital_out.go`, `project/mcu_pwm.go`, and `project/mcu_adc.go` hardware/config shells, bus and driver code, and the core printer runtime/bootstrap shell.

## Pause-Point Checkpoint (2026-04-05)

If the migration pauses here so unrelated module work can land first, the current resume point is well-defined:

- recent landed migration commits immediately before the current working slice include:
   - `b843f9d` — move toolhead batch scheduler into `internal/pkg/motion`
   - `ad39dfc` — move extruder motion policy into `internal/pkg/motion`
   - `33412c2` — move toolhead planner into `internal/pkg/motion`
- the repository currently passes `go test ./...`
- the main binary currently builds with `go build ./cmd/gklib`
- `gklib` is therefore ready for smoke/regression testing before the migration is fully complete

The recommended resume order after the module work lands is:

1. finish the remaining `project/toolhead_*.go` queue/trapq/bootstrap shell extraction
2. continue locking the `project/mcu_stepper.go` + `project/printer_rail.go` / `project/mcu_*.go` lifecycle boundary now that the timing, stepper-position, and trsync runtime helpers are internalized
3. move the remaining probe/bed-mesh/report/manual-probe shells still living in `project/`
4. then migrate TMC, bus, and sensor families

## Missing Target Areas in the Current Plan

The original directory sketch is still directionally correct, but it is missing several concrete destinations that are clearly needed by the remaining `project/` code:

- `internal/pkg/motion/kinematics/` — cartesian, corexy, extruder, none
- `internal/pkg/motion/homing/` — homing workflow, home-rails state machine
- `internal/pkg/motion/bed_mesh/` — bed mesh and adaptive bed mesh
- `internal/pkg/motion/report/` — motion status, trapq/stepper dumps
- `internal/pkg/gcode/macro.go` — macro/template execution
- `internal/pkg/reactor/` or an equivalent long-term home for the event-loop facade if `Reactor.go` is eventually migrated
- a decided home for ACE/K3C-specific integrations, likely `internal/addon/ace/` or `internal/hardware/ace/`

The plan also needs to explicitly acknowledge that `toolhead` is still a major unresolved boundary. It may end up as `internal/pkg/toolhead/`, or it may remain grouped under `internal/pkg/motion/` as the composition point for kinematics and move scheduling.

## Remaining Work Buckets

The remaining effort is not evenly distributed. By file count the migration is already well past halfway, but by engineering risk the repository is still in the middle of the work because the hardest subsystems are still in `project/`.

| Bucket | Main examples still in `project/` | Estimated effort |
|---|---|---:|
| Thin adapters and cleanup | small glue files, remaining wrappers, already-internal-backed duplicates | 1–3 days |
| G-code/macros and smaller utilities | `extras_gcode_macro.go`, remaining command glue | 2–4 days |
| Motion backbone | remaining `project/toolhead_*.go` queue/trapq/bootstrap shell, plus `project/mcu_stepper.go` / `project/printer_rail.go` and `project/mcu_*.go` lifecycle, `project/mcu_endstop.go` config/registration shell, and `project/mcu_trsync.go` config/stop ownership | 7–11 days |
| Homing, probe, bed mesh, motion reporting | remaining shells in `extras_homing.go`, `extras_probe.go`, `extras_manual_probe.go`, `extras_bed_mesh.go`, `extras_motion_report.go` | 6–10 days |
| Bus, TMC, and sensor drivers | `extras_bus.go`, `extras_tmc*.go`, `extras_accelerometer.go`, `extras_lis2dw12.go` | 10–15 days |
| Core runtime/bootstrap and board-specific integration | `k3c.go`, `Reactor.go`, `configfile.go`, `Webhooks.go`, ACE/K3C integration files | 8–12 days |

### Practical completion estimate

- **By structure/file count:** roughly **76–80% complete**
- **By risk-weighted engineering effort:** roughly **66–70% complete**

The difference is important: the easy compatibility and registration slices are mostly done, while the remaining work is concentrated in a smaller number of highly-coupled files.

### Binary readiness at this checkpoint

- **Current state:** the repository is buildable and testable enough to validate the current `gklib` binary
- **Not yet true:** the architecture is not yet stable enough to treat feature work as fully de-risked on top of the still-moving motion/toolhead/runtime boundaries

## Revised Migration Phases

### Phase 0 — Quick wins and cleanup

Target duration: **1–2 weeks**

- remove remaining thin adapters and duplicate wrappers
- finish moving already-internal-backed helpers out of `project/`
- move macro and other low-coupling G-code utilities into `internal/pkg/gcode`

### Phase 1 — Lock the motion interfaces

Target duration: **1–2 weeks**

- define the stable boundaries for `Toolhead`, kinematics, and stepper interaction
- create the target package for kinematics (`internal/pkg/motion/kinematics/`)
- move `kinematics_cartesian.go`, `kinematics_corexy.go`, `kinematics_none.go`, and the extruder kinematics pieces behind that boundary

This phase is the key unlock for most of the remaining migration.

### Phase 2 — Move homing/probe/mesh onto the new motion boundary

Target duration: **2–3 weeks**

- migrate homing workflow
- migrate probe and manual probe code
- migrate bed mesh and adaptive bed mesh
- migrate motion reporting/debug helpers

### Phase 3 — Migrate hardware buses and driver families

Target duration: **2–3 weeks**

- move SPI/I2C bus support into the `internal/pkg/mcu` boundary
- migrate shared TMC infrastructure and individual driver variants
- migrate remaining accelerometer/sensor drivers that belong with `motion/vibration` or `hardware`

### Phase 4 — Reduce `project/` to the application shell

Target duration: **2–3 weeks**

- decide whether `Printer`, `Reactor`, `ConfigWrapper`, and `Webhooks` become internal packages or remain as the final application shell
- decide where ACE/K3C-specific integrations live long term
- remove or minimize the remaining `project/` package to lifecycle/composition code only

## Total Remaining Effort

Assuming one engineer already familiar with the codebase:

- **To get `project/` down to a thin application shell:** about **18–28 engineering days**
- **To move essentially all substantive code out of `project/`:** about **28–40 engineering days**

Confidence level: **medium**

Main risks:

- `Toolhead` / kinematics boundary is still unresolved
- `Reactor` and printer lifecycle code are deeply cross-cutting
- MCU command queue ownership and stepper timing bugs can turn small refactors into hardware-debug sessions
- ACE/K3C-specific code may be intentionally better kept as a final board-integration layer instead of being forced into the same package pattern as generic printer logic

## Recommended Near-Term Sequence

If the goal is to keep momentum high and continue taking clean slices, the next order of attack should be:

1. finish the remaining `project/toolhead_*.go` queue/trapq/bootstrap shell extraction
2. continue carving `project/mcu_stepper.go`, `project/printer_rail.go`, and the remaining `project/mcu_*.go` shell so only lifecycle, `MCU_endstop` config/registration shells, trsync config/stop shells, and serial ownership stay in `project/`
3. move the remaining probe/bed-mesh/report/manual-probe shells onto the now-stronger motion boundary
4. then migrate TMC/bus/sensor drivers
5. only after that, decide how much of `k3c.go`, `Reactor.go`, `configfile.go`, and ACE-specific code should truly leave `project/`

This keeps the repository moving while deferring the highest-risk lifecycle refactors until the internal package boundaries are already strong.

## Project Directory Retirement Priority Map (2026-04-09)

If the goal shifts from “keep taking any safe slice” to “empty `project/` as quickly as possible”, the remaining `project/*.go` files should be treated as a dependency-ordered retirement plan rather than a flat backlog. The seven priority bands below cover all **69** current Go files in `project/` exactly once.

Two execution rules make this plan faster in practice:

1. **Run P0 and P1 as parallel cleanup tracks** whenever a small, low-risk commit is available.
2. **Treat P2 as the strategic unlock**. Until the toolhead/kinematics/stepper boundary is stable, `project/` can shrink, but it cannot disappear.

### P0 — Parallel cleanup of generic compatibility wrappers

These files are small, mostly forwarding-only, and should be absorbed into the owning shell or the eventual destination package whenever a nearby refactor touches them.

- Files:
   - `importlib.go`
   - `module_config_compat.go`
   - `must_lookup_method.go`
   - `printer_runtime_reactor_adapter.go`
   - `serial_reactor_adapter.go`
- Likely destinations:
   - `internal/pkg/printer`
   - `internal/pkg/reactor` or the final runtime shell
   - `internal/pkg/web` / `internal/pkg/serialhdl`
- Why this band exists: these files do not block the hard motion or MCU work, but they are easy file-count wins and should not linger once their callers move.

### P1 — Small module shells already mostly backed by `internal/`

These files mostly hold config parsing, concrete device ownership, or thin product/runtime glue around logic that already lives in `internal/pkg/...` or `internal/print`.

- Files:
   - `extras_gcode_macro.go`
   - `extras_adc_scaled.go`
   - `extras_extruder_stepper.go`
   - `extras_manual_stepper.go`
   - `extras_adxl345.go`
   - `extras_lis2dw12.go`
   - `extras_leviq3.go`
   - `extras_leviq3_test.go`
   - `extras_homing_heaters.go`
- Likely destinations:
   - `internal/pkg/gcode`
   - `internal/pkg/mcu`
   - `internal/pkg/motion`
   - `internal/pkg/motion/vibration`
   - `internal/print`
   - `internal/pkg/heater`
- Why this band exists: these modules are not the deepest architectural blockers, but removing them quickly reduces the surface area of `project/` and makes later backbone work easier to reason about.

### P2 — Motion backbone unlock (highest strategic priority)

This is the true critical path. Until this band is reduced, `project/` remains the owner of the motion scheduler, rail/stepper composition, and the kinematics boundary.

- Files:
   - `toolhead_module.go`
   - `toolhead_runtime_shell.go`
   - `toolhead_runtime_adapters.go`
   - `toolhead_commands.go`
   - `kinematics_interface.go`
   - `kinematics_adapters.go`
   - `kinematics_extruder.go`
   - `mcu_stepper.go`
   - `printer_rail.go`
- Likely destinations:
   - `internal/pkg/motion`
   - `internal/pkg/motion/kinematics`
   - a dedicated `internal/pkg/toolhead` package if that boundary is chosen
   - `internal/pkg/mcu` for any remaining stepper/rail helper seams
- Why this band exists: every remaining major shell depends on this runtime graph. Reducing file count elsewhere helps, but resolving this band is what unlocks the eventual retirement of `project/` as a whole.

### P3 — Homing, probe, bed-mesh, and motion-report shells

Once P2 is stable, these files can collapse quickly because much of their logic already lives in `internal/pkg/motion/*` packages and they will no longer need to bridge across a moving toolhead/rail boundary.

- Files:
   - `extras_homing.go`
   - `homing_adapters.go`
   - `extras_probe.go`
   - `probe_adapters.go`
   - `extras_manual_probe.go`
   - `manual_probe_adapters.go`
   - `extras_bed_mesh.go`
   - `bed_mesh_adapters.go`
   - `extras_adaptive_bed_mesh.go`
   - `extras_adaptive_bed_mesh_test.go`
   - `extras_motion_report.go`
   - `motion_report_adapters.go`
   - `extras_axis_twist_compensation.go`
   - `probe_command_adapters_test.go`
- Likely destinations:
   - `internal/pkg/motion/homing`
   - `internal/pkg/motion/probe`
   - `internal/pkg/motion/bed_mesh`
   - `internal/pkg/motion/report`
   - `internal/pkg/motion/kinematics` for any leftover compensation helpers
- Why this band exists: these files are now mostly orchestration and config shells sitting on top of internal motion packages. They should move soon after the motion backbone stops moving under them.

### P4 — MCU lifecycle, pin, and bus shell

This band owns the remaining concrete MCU runtime objects, command queues, pin registration, and synchronized endstop/trigger plumbing. It should move only after P2 and most of P3 are under control, because those layers still depend on concrete MCU-owned side effects.

- Files:
   - `mcu_core.go`
   - `mcu_connection.go`
   - `mcu_runtime.go`
   - `mcu_lifecycle.go`
   - `mcu_endstop.go`
   - `mcu_trsync.go`
   - `mcu_adc.go`
   - `mcu_digital_out.go`
   - `mcu_pwm.go`
   - `mcu_timing_adapters.go`
   - `extras_bus.go`
   - `pins.go`
- Likely destinations:
   - `internal/pkg/mcu`
   - `internal/pkg/printer`
   - a small hardware-facing runtime package if the existing `mcu` boundary becomes too broad
- Why this band exists: this is where serial ownership, OID allocation, pin registration, and concrete MCU lifecycle state still live. It is one of the final hard boundaries, not an early cleanup target.

### P5 — TMC driver and transport family

These files are downstream of the MCU shell and should be moved after the MCU transport/pin ownership story is much thinner. They are not the first blocker, but they are one of the last medium-to-high coupling clusters.

- Files:
   - `driver_adapters_tmc.go`
   - `tmc_spi_chain.go`
   - `tmc_spi_transport.go`
   - `tmc_uart_bitbang.go`
   - `tmc_uart_mux.go`
   - `tmc_uart_transport.go`
- Likely destinations:
   - `internal/pkg/tmc`
   - `internal/pkg/mcu`
   - a small hardware transport layer if TMC transport ownership should remain separate from generic MCU support
- Why this band exists: the shared TMC logic is already inside `internal/pkg/tmc`; what remains in `project/` is mostly concrete transport, config, and event wiring that depends on the MCU shell.

### P6 — Final application shell and board/product integration

This is the last band on purpose. Even after most logic has moved inward, some of these files may intentionally remain as the final composition shell unless the repository chooses to rename that shell rather than merely empty it.

- Files:
   - `k3c.go`
   - `Reactor.go`
   - `Webhooks.go`
   - `configfile.go`
   - `gcode.go`
   - `console.go`
   - `module_bootstrap_adapters.go`
   - `hub_feed.go`
   - `hub_unwind.go`
   - `extras_ace.go`
   - `extras_ace_commun.go`
   - `extras_ace_commun_proxy.go`
   - `ace_serial_port_linux.go`
   - `ace_serial_port_other.go`
- Likely destinations:
   - `internal/pkg/printer`
   - `internal/pkg/reactor`
   - `internal/pkg/web`
   - `internal/addon/ace` or `internal/hardware/ace`
   - platform-specific runtime packages for ACE serial support
- Why this band exists: these files are the true application shell, platform integration, and Anycubic/K3C-specific composition layer. Some of them may be the final survivors even after `project/` becomes a tiny shell.

### Practical scheduling consequence

If the objective is **raw file-count reduction**, keep landing P0 and P1 in parallel.

If the objective is **actually eliminating `project/`**, spend architecture effort in this order:

1. **P2 motion backbone unlock**
2. **P3 homing/probe/mesh/report shells**
3. **P4 MCU shell**
4. **P5 TMC transport/driver shell**
5. **P6 final application and board-integration shell**

| Cycle | Target slice | Likely files | Exit condition |
|---|---|---|---|
| 1 | ✅ Extracted `TMCErrorCheck` runtime core behind printer/reactor adapters | `project/tmc_runtime.go`, `internal/pkg/tmc/error_check_runtime.go` | `project/tmc_runtime.go` keeps only event/timer registration, status wrapping, and adapter glue |
| 2 | ✅ Extracted `TMCCommandHelper` runtime/state bookkeeping behind G-code and stepper adapters | `project/tmc_runtime.go`, `internal/pkg/tmc/command_runtime.go` | register init/dump/state logic lives in `internal/pkg/tmc`; `project/` keeps only command registration and concrete callback wiring |
| 3 | ✅ Extracted `TMCVirtualPinHelper` homing runtime behind homing/pins adapters | `project/tmc_runtime.go`, `internal/pkg/tmc/virtual_pin_runtime.go` | homing-state transitions and virtual-endstop coordination live in `internal/pkg/tmc`; `project/` keeps pin setup and event payload adaptation |
| 4 | ✅ Collapsed the remaining `project/tmc_runtime.go` shell into the TMC driver adapter and transport files | `project/tmc_runtime.go`, `project/driver_adapters_tmc.go`, `project/tmc_transport_uart.go`, `project/spi_tmc_transport.go` | `project/tmc_runtime.go` is deleted; the driver adapter owns the remaining command/virtual-pin/config wiring and the transports use `internal/pkg/tmc` types directly |
| 5 | ✅ Extracted UART register-transaction and IFCNT confirmation core | `project/tmc_transport_uart.go`, `internal/pkg/tmc/uart_transport_runtime.go` | UART read/write sequencing, retry rules, and IFCNT bookkeeping now live in `internal/pkg/tmc`; `project/` keeps the raw MCU command wrappers, mux activation, and config callback ownership |
| 6 | ✅ Extracted UART shared-resource and mux coordination seam | `project/tmc_transport_uart.go`, `internal/pkg/tmc/uart_resource_runtime.go` | printer mutex lookup and mux-state coordination are now adapter-driven in `internal/pkg/tmc`; `project/` keeps MCU pin lookup, config callbacks, and concrete transport ownership |
| 7 | ✅ Minimized the remaining SPI transport shell and SPI-side driver adapter surface | `project/spi_tmc_transport.go`, `project/driver_adapters_tmc.go`, `internal/pkg/tmc/spi_transport_runtime.go` | shared SPI chain/read-select state now lives in `internal/pkg/tmc`, the SPI-side adapter surface is smaller, and remaining `project/` code is limited to raw MCU/SPI ownership plus thin register-access wrappers |
| 8 | ✅ Move remaining SPI/I2C runtime helpers out of `project/extras_bus.go` | `project/extras_bus.go`, `internal/pkg/mcu` | bus runtime helpers now align with the internal MCU boundary; `internal/pkg/mcu/bus_session_runtime.go` owns bus-name resolution and live SPI/I2C session helpers while `project/extras_bus.go` keeps only constructor/adapter shells |
| 9 | ✅ Move shared accelerometer/runtime logic into `internal/pkg/motion/vibration` | `project/extras_accelerometer.go`, `project/extras_adxl345.go`, `project/extras_lis2dw12.go`, `internal/pkg/motion/vibration` | `internal/pkg/motion/vibration/accelerometer_helpers.go` now owns the shared query, G-code, and clock-sync helpers; `project/` keeps the concrete SPI/bulk-query shells and `project/extras_accelerometer.go` is deleted |
| 10 | ✅ Move macro execution/runtime out of `project/extras_gcode_macro.go` and reassess the next backbone slice | `project/extras_gcode_macro.go`, `internal/pkg/gcode` | `internal/pkg/gcode/macro_module.go` now owns macro execution/runtime while `project/extras_gcode_macro.go` keeps the template host shell; the next concrete backbone slice is the remaining `project/toolhead.go` timer callback and drip-mode shell |
| 11 | ✅ Move the toolhead drip advance loop out of `project/toolhead.go` | `project/toolhead.go`, `internal/pkg/motion/toolhead_drip.go` | `internal/pkg/motion/toolhead_drip.go` now owns the drip advance loop and drip-end sentinel; `project/toolhead.go` keeps reactor timer/completion ownership, trapq finalization, and move-submit error handling while the next concrete slice is the timer/priming/flush callback shell |
| 12 | ✅ Move the toolhead pause-check application and timer callbacks out of `project/toolhead.go` | `project/toolhead.go`, `internal/pkg/motion/toolhead_timer_runtime.go` | `internal/pkg/motion/toolhead_timer_runtime.go` now owns pause-check application plus the priming/flush timer callback runtime; `project/toolhead.go` keeps concrete reactor timer handles, lookahead execution, and panic wrappers while the next concrete slice is the remaining move-submit, wait, and reactor-completion shell |
| 13 | ✅ Move the toolhead wait wrapper and drip-move orchestration out of `project/toolhead.go` | `project/toolhead.go`, `internal/pkg/motion/toolhead_move_runtime.go` | `internal/pkg/motion/toolhead_move_runtime.go` now owns the `Wait_moves` wrapper plus drip-mode entry/error/cleanup orchestration; `project/toolhead.go` keeps the concrete lookahead queue, trapq finalization, and move-submission shell while the next concrete slice is the remaining move-submission, lookahead-reset, and queue/trapq ownership shell |
| 14 | ✅ Delete `project/toolhead.go` after moving the remaining control helpers inward and splitting the surviving project shell | `project/toolhead.go`, `project/toolhead_*.go`, `internal/pkg/motion/toolhead_control_runtime.go` | `internal/pkg/motion/toolhead_control_runtime.go` now owns move/manual-move/dwell/velocity-limit control helpers, `project/toolhead.go` is deleted, and the remaining project-side toolhead shell is split across smaller `project/toolhead_*.go` files |
| 15 | ✅ Delete `project/stepper.go` by splitting the surviving stepper shell into focused project files | `project/stepper.go`, `project/mcu_stepper.go`, `project/printer_rail.go` | `project/stepper.go` is deleted, the remaining project-side stepper shell now lives in `project/mcu_stepper.go` and `project/printer_rail.go`, and the exported `MCU_stepper` / `PrinterRail` / `PrinterStepper` / `LookupMultiRail` APIs stay intact |
| 16 | ✅ Delete the old TMC SPI/UART transport files by splitting the surviving transport shell into focused project files | `project/spi_tmc_transport.go`, `project/tmc_transport_uart.go`, `project/tmc_spi_*.go`, `project/tmc_uart_*.go` | the old SPI/UART transport filenames are deleted, the remaining project-side TMC transport shell is split across smaller `project/tmc_spi_*.go` and `project/tmc_uart_*.go` files, and the public TMC transport constructors plus `RegisterAccess` APIs stay intact |
| 17 | ✅ Delete the legacy stepper shell filenames after the stepper split | `project/stepper_mcu.go`, `project/stepper_rail.go`, `project/mcu_stepper.go`, `project/printer_rail.go` | the old `project/stepper_mcu.go` and `project/stepper_rail.go` filenames are deleted, the surviving stepper shell is renamed to `project/mcu_stepper.go` and `project/printer_rail.go`, and the exported `MCU_stepper` / `PrinterRail` / `PrinterStepper` / `LookupMultiRail` APIs stay intact |
| 18 | ✅ Consolidate the thin probe adapter layer into one project file | `project/probe_adapters.go`, `project/probe_command_adapters.go`, `project/probe_endstop_adapters.go`, `project/probe_event_adapters.go`, `project/probe_motion_adapters.go`, `project/probe_points_adapters.go`, `project/probe_run_adapters.go` | the package-private probe bridge types now live together in `project/probe_adapters.go`, the six old per-adapter filenames are deleted, and `project/extras_probe.go` keeps the same call sites and behavior |
| 19 | ✅ Consolidate the remaining tiny manual-probe and motion-report helper adapter files | `project/manual_probe_adapters.go`, `project/manual_probe_command_adapters.go`, `project/motion_report_adapters.go`, `project/api_dump_adapters.go` | the package-private manual-probe bridge types now live together in `project/manual_probe_adapters.go`, the motion-report helper adapters now live together in `project/motion_report_adapters.go`, the two old helper filenames are deleted, and `project/extras_manual_probe.go` / `project/extras_motion_report.go` keep the same call sites and behavior |
| 20 | ✅ Move MCU stepper homing-reset and position-query orchestration out of `project/mcu_stepper.go` | `project/mcu_stepper.go`, `internal/pkg/mcu/stepper_position_runtime.go` | `internal/pkg/mcu/stepper_position_runtime.go` now owns the post-homing reset plus MCU-position query/sync orchestration, `project/mcu_stepper.go` keeps the concrete stepper shell and event wiring, and direct `serialhdl` knowledge is removed from the project-side stepper shell |
| 21 | ✅ Move rail position/min/max and homing settings resolution out of `project/printer_rail.go` | `project/printer_rail.go`, `internal/pkg/motion/kinematics/rail_settings.go` | `internal/pkg/motion/kinematics/rail_settings.go` now owns the legacy rail position-endstop selection plus range/homing-settings resolution and contextual validation errors, while `project/printer_rail.go` keeps only the concrete stepper/endstop ownership and attachment shell |
| 22 | ✅ Move rail endstop reuse/create/query-registration orchestration out of `project/printer_rail.go` | `project/printer_rail.go`, `internal/pkg/mcu/rail_endstop_runtime.go` | `internal/pkg/mcu/rail_endstop_runtime.go` now owns the legacy rail endstop lookup, reuse/new-endstop decision, conflict detection, and query-endstop registration orchestration, while `project/printer_rail.go` keeps the concrete pin parsing, endstop object storage, and stepper attachment shell |
| 23 | ✅ Move `PrinterStepper` config/pin-resolution and helper-module registration sequencing out of `project/mcu_stepper.go` | `project/mcu_stepper.go`, `internal/pkg/mcu/legacy_stepper_factory.go` | `internal/pkg/mcu/legacy_stepper_factory.go` now owns the legacy stepper plan building plus helper-module registration sequencing, while `project/mcu_stepper.go` keeps concrete `MCU_stepper` construction and the module-specific type assertions |
| 24 | ✅ Move reusable extruder runtime/status behavior out of `project/kinematics_extruder.go` | `project/kinematics_extruder.go`, `project/kinematics_interface.go`, `internal/pkg/motion/extruder_runtime.go` | `internal/pkg/motion/extruder_runtime.go` now owns the legacy extruder runtime/status shell plus dummy extruder behavior, `project/kinematics_interface.go` now aliases the internal extruder contract, and `project/kinematics_extruder.go` keeps heater setup, extruder-stepper ownership, and G-code command wiring |
| 25 | ✅ Route toolhead extruder move dispatch through the shared internal contract | `project/toolhead_runtime_adapters.go`, `project/toolhead_runtime_adapters_test.go`, `internal/pkg/motion/extruder_runtime.go` | the shared `motion.Extruder` contract now exposes `Move`, `project/toolhead_runtime_adapters.go` no longer type-asserts to `*PrinterExtruder`, and the project-side adapter path has a focused regression test |
| 26 | ✅ Move initial toolhead velocity-limit planning into `internal/pkg/motion` | `project/toolhead_module.go`, `internal/pkg/motion/toolhead_config.go` | `internal/pkg/motion/toolhead_config.go` now owns initial toolhead velocity/junction planning, while `project/toolhead_module.go` keeps config reads and module bootstrap |
| 27 | ✅ Move toolhead command handling into `internal/pkg/motion` | `project/toolhead_commands.go`, `project/toolhead_runtime_shell.go`, `internal/pkg/motion/toolhead_commands.go` | `internal/pkg/motion/toolhead_commands.go` now owns G4/M400/SET_VELOCITY_LIMIT/M204 handling plus acceleration updates, while `project/toolhead_commands.go` keeps thin G-code and printer-state adapters and the now-dead `Calc_junction_deviation` wrapper is removed |
| 28 | ✅ Move scaled-ADC initialization/orchestration into `internal/pkg/heater` | `project/extras_adc_scaled.go`, `internal/pkg/heater/adc_scaled_module.go` | `internal/pkg/heater/adc_scaled_module.go` now owns scaled-ADC chip initialization/orchestration, while `project/extras_adc_scaled.go` keeps the concrete MCU-pin and virtual-chip registration shell |
| 29 | ✅ Move shared accelerometer clock-update sequencing into `internal/pkg/motion/vibration` | `project/extras_adxl345.go`, `project/extras_lis2dw12.go`, `internal/pkg/motion/vibration/chip_clock_sync.go` | `internal/pkg/motion/vibration/chip_clock_sync.go` now owns FIFO/status clock-update sequencing, while the project-side accelerometer shells keep the concrete status-query and bulk-read ownership |

