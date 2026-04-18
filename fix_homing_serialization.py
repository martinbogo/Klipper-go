#!/usr/bin/env python3
"""Fix virtual pin homing serialization by adding time/sync imports and delay logic."""

import re

# Read the original file
with open('internal/pkg/tmc/driver_helper_runtime.go', 'r') as f:
    content = f.read()

# Step 1: Add imports if not present
import_block_pattern = r'import \(\s*"errors".*?"strings"\s*\)'
import_replacement = '''import (
	"errors"
	"fmt"
	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	"goklipper/common/utils/str"
	"goklipper/common/value"
	"strings"
	"sync"
	"time"
)'''

content = re.sub(import_block_pattern, import_replacement, content, flags=re.DOTALL)

# Step 2: Add var block after const block
const_block_pattern = r'(const \(\s*cmd_INIT_TMC_help.*?cmd_DUMP_TMC_help.*?\n\))'
var_block = '''var (
	homingMoveSequenceLock sync.Mutex
	homingMoveSequenceTime time.Time
	homingMoveSequenceCount int
)

'''
content = re.sub(const_block_pattern, r'\1\n\n' + var_block, content, flags=re.DOTALL)

# Step 3: Replace handle_homing_move_begin function
old_begin = r'func \(self \*DriverVirtualPinHelper\) handle_homing_move_begin\(argv \[\]interface\{\}\) error \{\s*return HandleVirtualPinHomingMoveBegin\(&driverVirtualPinEventRuntime\{helper: self\}, self\.moveEndstops\(argv\)\)\s*\}'

new_begin = '''func (self *DriverVirtualPinHelper) handle_homing_move_begin(argv []interface{}) error {
	// Serialize virtual pin homing setup across multiple drivers sharing one UART.
	// Multiple motors calling BeginHoming() in rapid succession write GCONF,
	// thresholds, and other registers. Add 75ms delay for 2nd/3rd drivers to allow
	// 1st driver's writes to complete on the shared bitbang UART.
	homingMoveSequenceLock.Lock()
	now := time.Now()
	// Reset counter if this is a new homing event (50ms since last call)
	if homingMoveSequenceTime.IsZero() || now.Sub(homingMoveSequenceTime) > 50*time.Millisecond {
		homingMoveSequenceCount = 0
		homingMoveSequenceTime = now
	} else {
		homingMoveSequenceCount++
	}
	delayNeeded := homingMoveSequenceCount > 0
	homingMoveSequenceLock.Unlock()
	
	// Add delay for 2nd and 3rd motors to prevent UART collisions
	if delayNeeded {
		time.Sleep(75 * time.Millisecond)
	}
	
	return HandleVirtualPinHomingMoveBegin(&driverVirtualPinEventRuntime{helper: self}, self.moveEndstops(argv))
}'''

content = re.sub(old_begin, new_begin, content, flags=re.DOTALL)

# Step 4: Replace handle_homing_move_end function  
old_end = r'func \(self \*DriverVirtualPinHelper\) handle_homing_move_end\(argv \[\]interface\{\}\) error \{\s*return HandleVirtualPinHomingMoveEnd\(&driverVirtualPinEventRuntime\{helper: self\}, self\.moveEndstops\(argv\)\)\s*\}'

new_end = '''func (self *DriverVirtualPinHelper) handle_homing_move_end(argv []interface{}) error {
	// Use similar serialization for EndHoming to prevent UART collisions.
	// Reset sequence counter when homing move ends.
	homingMoveSequenceLock.Lock()
	homingMoveSequenceTime = time.Time{}  // Reset for next homing event
	homingMoveSequenceLock.Unlock()
	
	return HandleVirtualPinHomingMoveEnd(&driverVirtualPinEventRuntime{helper: self}, self.moveEndstops(argv))
}'''

content = re.sub(old_end, new_end, content, flags=re.DOTALL)

# Write back
with open('internal/pkg/tmc/driver_helper_runtime.go', 'w') as f:
    f.write(content)

print("Successfully applied homing serialization fix")
