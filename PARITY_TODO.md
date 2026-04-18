# Parity TODO

## Phase 1: Motion queue and flush parity (highest impact)

1. Port `motion_queuing.py` flush state-machine behavior
   - Reproduce active-stepping versus relaxed flushing paths.
   - Match thresholds and branch conditions.

2. Add active stepping flush timing constants and flow
   - Implement `BGFLUSH_SG_*` equivalents in Go.
   - Keep existing defaults only where parity confirms.

3. Match flush wakeup/reschedule policy
   - Align timer wake computations and batching behavior.
   - Ensure `need_flush_time` and `need_step_gen_time` interactions match vanilla.

4. Add parity tests for flush timer behavior
   - Deterministic tests for callback cadence and next wake time.
   - Cases for active stepping and non-stepping transitions.

## Phase 2: Lookahead and planner semantics

5. Audit lookahead reverse-pass and delayed propagation
   - Verify cutoff logic, pending assignments, and lazy flush behavior.

6. Add golden tests for junction boundary cases
   - Corner-angle sweeps and mixed accel limits.
   - Compare computed `start/cruise/end` profiles against vanilla fixtures.

7. Canonicalize `minimum_cruise_ratio` semantics
   - Make `minimum_cruise_ratio` the primary internal concept.
   - Keep behavior numerically equivalent.

8. Maintain `max_accel_to_decel` compatibility alias
   - Continue accepting/reporting it for compatibility.
   - Ensure conversion is symmetric and tested.

## Phase 3: Homing and TMC sequencing parity

9. Verify homing lifecycle sequencing parity
   - Confirm ordering and timing of home begin/end, retract, and final position updates.

10. Tighten TMC suppression scope and grace windows
    - Restrict suppression to homing-critical windows.
    - Reduce chance of hiding non-transient faults.

11. Refine virtual-pin serialization policy
    - Replace fixed-delay behavior with readiness-based pacing where possible.
    - Preserve shared-UART stability guarantees.

## Phase 4: Validation and regression protection

12. Create golden A/B parity fixtures
    - Trapq append timeline, flush wakeups, queue depth transitions, homing outcomes.

13. Build motion telemetry diff tooling
    - Segment timing, flush cadence, queue depth, and step generation deadlines.

14. Add shared-UART contention stress regressions
    - Homing with multi-driver activity.
    - Assertions on retry bounds, no shutdown, stable phase sync.

15. Run live A/B hardware validation
    - Same config and gcode path on vanilla versus Go.
    - Record and compare smoothness-sensitive metrics before and after each batch.

## Suggested execution order

- PR 1: Items 1-4
- PR 2: Items 5-8
- PR 3: Items 9-11
- PR 4: Items 12-15
