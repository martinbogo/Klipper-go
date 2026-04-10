# Anycubic Color Engine Pro (ACE) - Integration & Implementation Specification

## Overview
This specification details the end-to-end integration between the Anycubic touchscreen UI (`K3SysUi`) and the custom Klipper-go backend (`gklib`). It covers how to spoof missing proprietary Anycubic hardware capabilities (custom filament metadata such as colors and types), enforce UI JSON-RPC schema requirements, persist user-defined filament attributes, and resolve data aliasing bugs within Go's diff engine.

If you are a developer, coding agent, or maintainer taking over this module, this file explains exactly **why** the code takes the shape it does, and provides enough detail to independently re-implement the UDS interceptions and serial communication from scratch.

---

## 1. Hardware vs. Software Source of Truth

The physical ACE hardware (`/dev/ttyACM0`) manages real-world physical telemetry: runout endstops, extruder feed states, heater dryer temperatures, and proprietary RFID tags. However, the physical hardware **does not locally store custom spool metadata**.

When a user manually configures a spool (e.g., "Red PA") via the touchscreen, the hardware does not record it. The backend software (`Klipper-go / gklib`) must act as a "virtual memory" unified state manager for non-RFID filaments:
1. Intercept custom configurations set via the UI.
2. Store the type/color data both in RAM (`custom_slots`) **and persist dynamically to disk**.
3. Merge this virtual metadata into the physical telemetry packets fetched from the ACE serial connection, so the UI believes the hardware natively reported the custom spool.

### 1.1 The File System Configuration (`ams_config.cfg`)

**Path:** `/userdata/app/gk/config/ams_config.cfg`

This JSON file is the centralized source of truth for custom spool metadata. Its exact schema:

```json
{
  "filaments": {
    "0": {
      "type": "PLA",
      "color": [255.0, 0.0, 0.0],
      "colors": [[255.0, 0.0, 0.0, 255]],
      "icon_type": 0
    },
    "1": { "type": "", "color": [0.0, 0.0, 0.0], "colors": [[0.0, 0.0, 0.0, 0]], "icon_type": 0 },
    "2": { "type": "", "color": [0.0, 0.0, 0.0], "colors": [[0.0, 0.0, 0.0, 0]], "icon_type": 0 },
    "3": { "type": "", "color": [0.0, 0.0, 0.0], "colors": [[0.0, 0.0, 0.0, 0]], "icon_type": 0 }
  }
}
```

**Field details:**
- Keys `"0"` through `"3"` are slot indices as strings.
- `type` (string): filament material name, e.g. `"PLA"`, `"ABS"`, `"PETG"`. Empty string for unconfigured.
- `color` (`[]float64`): `[R, G, B]` with values `0.0`-`255.0`. Go stores these as `[]interface{}` of `float64`.
- `colors` (`[][]float64`): `[[R, G, B, A]]` -- the alpha-extended version. If only RGB is provided, the backend constructs this as `[[R, G, B, 255]]`.
- `icon_type` (int): always `0` for custom filaments (solid color bucket rendering).

**Lifecycle:**
- On boot: `extras_ace.go` reads this file and pre-populates `self.custom_slots`.
- On update: `Set_filament_info()` reads the file, updates the relevant slot entry, and writes it back (mode `0644`).

---

## 2. The UDS (Unix Domain Socket) Client-Server Contract

The touchscreen UI (`K3SysUi`) is a strictly-typed JSON-RPC client.

- **Socket Path**: `/tmp/klippy_uds` (configurable via `[apiserver]` config section)
- **Transport**: `0x03`-byte terminated JSON strings. Each message is a complete JSON object followed by a single `0x03` byte.
- **Partial buffering**: The server accumulates bytes until `0x03` is encountered, then parses the accumulated buffer as a JSON message.

### 2.1 JSON-RPC Request Format

```json
{
  "id": 1234.0,
  "method": "filament_hub/set_filament_info",
  "params": { "index": 0, "type": "PLA", "color": { "R": 255, "G": 0, "B": 0 } }
}
```

- `id` (float64): unique request identifier, echoed back in the response.
- `method` (string): endpoint path.
- `params` (object): method-specific parameters.

### 2.2 JSON-RPC Response Format

**Success:**
```json
{ "id": 1234.0, "result": { ... } }
```

**Error:**
```json
{ "id": 1234.0, "error": { "type": "error_type", "code": 500, "message": "description" } }
```

### 2.3 Registered Filament Hub Endpoints

All registered in `NewWebHooks()` via `Register_endpoint(path, callback)`:

| Endpoint | Direction | Description |
|---|---|---|
| `filament_hub/info` | GET | Returns hardware identity (model, firmware, serial number, slot count) |
| `filament_hub/query_version` | GET | Returns boot and firmware version strings |
| `filament_hub/filament_info` | GET | Returns slot data for a given index; reads from `ams_config.cfg` |
| `filament_hub/set_filament_info` | POST | User updates a custom spool; triggers disk write + RAM update |
| `filament_hub/get_config` | GET | Returns AMS config (auto_refill, flush_volume, runout_detect) |
| `filament_hub/set_config` | POST | Persists ace_* variables via `SAVE_VARIABLE` GCode |
| `filament_hub/start_drying` | POST | Starts the ACE dryer |
| `filament_hub/stop_drying` | POST | Stops the ACE dryer |

*Note: The UI executes actual physical feeds and rewinds by sending native G-Code commands (`FEED_FILAMENT`, `UNWIND_FILAMENT`) directly to the G-Code parser, rather than via JSON-RPC endpoints. `start_drying` and `stop_drying` are supported via JSON-RPC, but can also be executed via GCode equivalents `ACE_START_DRYING` and `ACE_STOP_DRYING`.*

### 2.3.1 G-Code Protocol Commands

In addition to the JSON-RPC endpoints handled natively by Webhooks, the ACE firmware/daemon provides several interactive G-Code macros for commandline and UI control.

#### Dryer Controls
- **`ACE_START_DRYING TEMP=<temp> DURATION=<minutes>`**: Starts the active filament dryer for the specified time and temperature. Max temperature is defined in `printer.cfg`.
- **`ACE_STOP_DRYING`**: Immediately halts the drying heater and fan.

#### Filament Operations (Feed & Unwind)
- **`FEED_FILAMENT INDEX=<spool_index> [LENGTH=<length>] [SPEED=<speed>]`**: Feeds the filament from the selected spool to the toolhead. Defaults to `toolchange_load_length` (typically 100mm) and feed speed (50mm/s).
- **`UNWIND_FILAMENT [INDEX=<spool_index>] [LENGTH=<length>] [SPEED=<speed>]`**: Retracts filament from the toolhead back into the ACE hub. Defaults to current active index if not provided, and uses `toolchange_retract_length` (100mm) and speed (50mm/s).
*Note: Underneath the hood, these map to the primitive `ACE_FEED` and `ACE_RETRACT` serial RPCs.*

#### Endless Spool (Filament Backup)
- **`ACE_ENABLE_ENDLESS_SPOOL`**: Enables the auto-swap continuity feature (Filament Backup) where an empty spool seamlessly transitions to a matching spool of the same material and color.
- **`ACE_DISABLE_ENDLESS_SPOOL`**: Disables the Filament Backup feature.
- **`ACE_ENDLESS_SPOOL_STATUS`**: Queries current feature status.

### 2.4 Endpoint Schemas

**`filament_hub/info` response:**
```json
{
  "infos": [{
    "id": 0,
    "slots": 4,
    "model": "Anycubic Color Engine Pro",
    "firmware": "V1.3.863"
  }]
}
```

**`filament_hub/query_version` response:**
```json
{ "boot_version": "V1.0.1", "version": "V1.3.863", "id": 0 }
```

**`filament_hub/filament_info` request params and response:**
```json
// Request params:
{ "index": 0 }

// Response (read from ams_config.cfg):
{
  "type": "PLA",
  "color": [255.0, 0.0, 0.0],
  "colors": [[255.0, 0.0, 0.0, 255]],
  "index": 0,
  "brand": "",
  "rfid": 1,
  "sku": "",
  "remain": 0,
  "icon_type": 0,
  "source": 2,
  "decorder": 0
}
```

**`filament_hub/set_filament_info` request params:**
```json
{
  "index": 0,
  "type": "PLA",
  "color": { "R": 255, "G": 0, "B": 0 }
}
```
The webhook handler extracts `R`, `G`, `B` as `float64` from the `color` map, converts to `[]interface{}{R, G, B}`, then calls `ace.Set_filament_info(index, type, "", color)` via the `FilamentInfoSetter` interface.

**`filament_hub/get_config` response:**
```json
{
  "auto_refill": 0,
  "flush_multiplier": 1.5,
  "flush_volume_min": 107,
  "flush_volume_max": 800,
  "runout_detect": 1
}
```

**`filament_hub/set_config` request (e.g., Toggling Filament Backup):**
When you toggle properties like "Filament Backup" (auto-refill) on or off, the UI sends this RPC command with the relevant parameter key.

*Enabling Filament Backup:*
```json
{
  "method": "filament_hub/set_config",
  "params": {
    "auto_refill": 1
  },
  "id": 84
}
```

*Disabling Filament Backup:*
```json
{
  "method": "filament_hub/set_config",
  "params": {
    "auto_refill": 0
  },
  "id": 85
}
```
*Note: Any key passed in `params` (like `auto_refill` or `runout_detect`) will be persisted to `/userdata/app/gk/config/ams_config.cfg` and the daemon will subsequently run `SAVE_VARIABLE VARIABLE=ace_{key} VALUE={value}` in Klipper.*

**`filament_hub/start_drying` request:**
```json
{
  "method": "filament_hub/start_drying",
  "params": {
    "duration": 240, 
    "target_temp": 50
  },
  "id": 85
}
```
*Note: `duration` (or interchangeably `time`) is expected in minutes. `target_temp` is in Celsius.*

**`filament_hub/stop_drying` request:**
```json
{
  "method": "filament_hub/stop_drying",
  "params": {},
  "id": 86
}
```

**Executing G-Code Commands (Feeds, Retracts, etc.)**
While `start_drying` and `set_config` use bespoke `filament_hub/` namespaces, interactions like feeding or unwinding send standard G-Code execution requests to the backend (typically routed via Moonraker/gklib's G-Code parser or `gcode/script` equivalents):

```json
{
  "method": "printer.gcode.script",
  "params": {
    "script": "FEED_FILAMENT INDEX=0"
  },
  "id": 87
}
```
```json
{
  "method": "printer.gcode.script",
  "params": {
    "script": "UNWIND_FILAMENT INDEX=0"
  },
  "id": 88
}
```

*Observation:* Immediately after a feed/extrude action successfully finishes, the UI automatically issues an extruder cool-down command to power off the hotend heater via a standard `M104 S0` script or a JSON-RPC temperature equivalent:
```json
{
  "method": "printer.gcode.script",
  "params": {
    "script": "M104 S0"
  },
  "id": 89
}
```

---

## 3. Serial Communication Protocol (ACE Hardware)

### 3.1 Connection

- **Device**: `/dev/ttyACM0` (auto-detected via glob `/dev/ttyACM*`, first match)
- **Baud rate**: 230400 (configurable via `[filament_hub] v2_baud`; also supports 115200)
- **Read timeout**: 900 microseconds

### 3.2 Termios Configuration

Raw serial mode with no flow control:
- **Cleared input flags**: `IGNBRK | BRKINT | PARMRK | ISTRIP | INLCR | IGNCR | ICRNL | IXON`
- **Cleared output flags**: `OPOST`
- **Cleared local flags**: `ECHO | ECHONL | ICANON | ISIG | IEXTEN`
- **Control flags**: clear `CSIZE | PARENB | CRTSCTS`, set `CS8 | CREAD | CLOCAL`
- Buffers flushed immediately after opening with `TCIOFLUSH`.

### 3.3 Binary Frame Format

Every serial message (both V1 JSON and V2 protobuf) uses the same binary frame wrapper:

```
Offset  Size   Field
------  -----  -----
0       1      FRAME_START_1 = 0xFF
1       1      FRAME_START_2 = 0xAA
2       2      Payload length (uint16, little-endian)
4       N      Payload (V1: JSON bytes, V2: protobuf bytes)
4+N     2      CRC-16 (uint16, little-endian)
6+N     1      FRAME_END = 0xFE
```

Minimum frame size: 7 bytes (empty payload).

### 3.4 CRC-16 Algorithms

**IMPORTANT:** The codebase contains **two different CRC-16 implementations** depending on the code path:

**V1 CRC (used by `extras_ace_commun.go` for frame validation on read):**
```go
func _calc_crc(buf []byte) uint16 {
    var crc uint16 = 0xFFFF
    for i := 0; i < len(buf); i++ {
        data := uint16(buf[i])
        data ^= crc & 0xFF
        data ^= (data & 0x0F) << 4
        crc = ((data << 8) | (crc >> 8)) ^ (data >> 4) ^ (data << 3)
    }
    return crc
}
```
- Input: **payload bytes only** (not frame header, length, or end marker)
- Used in `Reader()` to validate incoming frames

**V2 CRC (Modbus, used by `ace_v2_proxy.go` for frame building and response parsing):**
```go
func calcCRC16(data []byte) uint16 {
    var crc uint16 = 0xFFFF
    for _, b := range data {
        crc ^= uint16(b)
        for i := 0; i < 8; i++ {
            if (crc & 1) != 0 {
                crc = (crc >> 1) ^ 0xA001
            } else {
                crc >>= 1
            }
        }
    }
    return crc
}
```
- Polynomial: `0xA001` (standard Modbus CRC-16)
- Input: **all bytes from frame start through end of payload** (i.e., `[0xFF, 0xAA, LenLo, LenHi, ...payload]`)
- Used in `BuildRequest()` and `ParseResponse()` in the V2 handler

Both CRCs use initial value `0xFFFF` and output little-endian, but they differ in polynomial and input scope.

### 3.5 V1 Protocol (Legacy JSON)

**Request (sent to ACE hardware):**
```json
{ "id": 42, "method": "get_status", "params": {} }
```

**Response (from ACE hardware):**
```json
{ "id": 42, "code": 0, "msg": "ok", "status": "ready", "slots": [...], ... }
```

The `id` is an auto-incrementing integer starting at 0, assigned by `AceCommun.Writer()`. Responses are matched to callbacks via `callback_map[id]`.

**V1 Serial Methods:**

| Method | Params | Response |
|---|---|---|
| `get_info` | `{}` | Full device info including firmware strings |
| `get_status` | `{}` | Current status, slot states, dryer state, temp |
| `drying` | `{temp: int, fan_speed: int, duration: int}` | `{code: 0, msg: "ok"}` |
| `drying_stop` | `{}` | `{code: 0}` |
| `start_feed_assist` | `{index: 0-3}` | `{code: 0}` |
| `stop_feed_assist` | `{index: 0-3}` | `{code: 0}` |
| `feed_filament` | `{index: 0-3, length: int, speed: int}` | `{code: 0}` |
| `unwind_filament` | `{index: 0-3, length: int, speed: int}` | `{code: 0}` |

### 3.6 V2 Protocol (Protobuf)

The V2 protocol replaces JSON payloads with protobuf-encoded binary messages inside the same `[0xFF, 0xAA, ..., 0xFE]` frame.

**IMPORTANT - Model-Based Protocol Routing:** The codebase dynamically detects which protocol to use based on the hardware model string returned by the `get_info` handshake. This is controlled by the `IsV2` flag on the `AceCommun` struct.

| Hardware Model String | Protocol | IsV2 |
|---|---|---|
| `"Anycubic Color Engine Pro"` | V1 (JSON) | `false` |
| `"Anycubic Color Engine Pro 2.0"` | V2 (Protobuf) | `true` |

Any model containing `"2.0"` activates V2 mode. All other models default to V1.

#### 3.6.0 Model Detection and Protocol Routing

On every successful serial connection, `_connect()` sends a `get_info` request. The response callback inspects the `model` field to set the protocol mode:

```go
// In extras_ace.go _connect():
self.send_request(map[string]interface{}{"method": "get_info"},
    func(response map[string]interface{}) {
        self.fw_info = response["result"].(map[string]interface{})
        if model, ok := self.fw_info["model"].(string); ok {
            if strings.Contains(model, "2.0") {
                self.ace_commun.IsV2 = true
            } else {
                self.ace_commun.IsV2 = false
            }
        }
    })
```

**Full `get_info` response example (V1 ACE Pro):**
```json
{
  "code": 0,
  "id": 0,
  "msg": "success",
  "result": {
    "boot_firmware": "V1.0.1",
    "firmware": "V1.3.863",
    "id": 0,
    "model": "Anycubic Color Engine Pro",
    "slots": 4,
    "structure_version": "0"
  }
}
```

The `IsV2` flag gates three code paths:

**1. Outbound request dispatch** (`extras_ace_commun_proxy.go`):
```go
func (self *AceCommun) _send_request(req map[string]interface{}) error {
    if !self.IsV2 {
        return self._send_request_v1(req)  // bypass proxy entirely
    }
    // ... V2 protobuf serialization switch ...
}
```
When `IsV2 == false`, every command (including `drying`, `get_status`, `feed`, etc.) is sent as native V1 JSON directly to the hardware. The protobuf serialization switch is never entered.

**2. Inbound response parsing** (`extras_ace_commun.go` `Reader()`):
```go
// Fast path: See if it's a V2 protobuf response
if self.IsV2 {
    unwrappedV2, errV2 := filament.NewV2ProtoHandler().ParseResponse(payload)
    if errV2 == nil && len(unwrappedV2) > 0 {
        ret = TranslateV2ResponseToJSON(unwrappedV2)
        // ... assign pseudo-ID from callback_map ...
    }
}
// Fall through to V1 JSON unmarshal if ret is still nil
if ret == nil {
    err := json.Unmarshal(payload, &ret)
}
```
When `IsV2 == false`, the protobuf parser is never invoked. All incoming frames are treated as JSON. This is critical because the V2 `ParseResponse` could misinterpret valid V1 JSON bytes as protobuf, silently consuming the response and producing garbage that breaks callback matching.

**3. Response ID extraction** (`extras_ace_commun.go` `Reader()`):
V1 JSON responses always contain an `"id"` field as a `float64`. The Reader uses a type-switch to handle both `float64` (V1) and `int` (V2 pseudo-ID) types:
```go
idVal, ok := ret["id"]
if !ok || idVal == nil {
    logger.Warnf("Response missing 'id', ignoring: %v", ret)
    return nil
}
var id int
switch v := idVal.(type) {
case float64:
    id = int(v)
case int:
    id = v
default:
    logger.Warnf("Response 'id' is not a number, ignoring: %v", ret)
    return nil
}
```

**Full V1 `get_status` response example (from ACE Pro hardware):**
```json
{
  "code": 0,
  "id": 3,
  "msg": "success",
  "result": {
    "cont_assist_time": 0,
    "dryer_status": {
      "duration": 0,
      "remain_time": 0,
      "status": "stop",
      "target_temp": 0
    },
    "enable_rfid": 1,
    "fan_speed": 7000,
    "feed_assist_count": 1,
    "slots": [
      {
        "color": [0, 0, 0],
        "colors": [[0, 0, 0, 0]],
        "icon_type": 0,
        "index": 0,
        "rfid": 1,
        "sku": "",
        "status": "ready",
        "type": ""
      },
      {
        "color": [0, 0, 0],
        "colors": [[0, 0, 0, 0]],
        "icon_type": 0,
        "index": 1,
        "rfid": 1,
        "sku": "",
        "status": "ready",
        "type": ""
      },
      {
        "color": [0, 0, 0],
        "colors": [[0, 0, 0, 0]],
        "icon_type": 0,
        "index": 2,
        "rfid": 1,
        "sku": "",
        "status": "ready",
        "type": ""
      },
      {
        "color": [106, 109, 205],
        "colors": [[106, 109, 205, 255]],
        "icon_type": 0,
        "index": 3,
        "rfid": 2,
        "sku": "AHPLPO-106",
        "status": "ready",
        "type": "PLA"
      }
    ],
    "status": "ready",
    "temp": 30
  }
}
```

**Full V1 `drying` request example (sent to ACE Pro hardware):**
```json
{
  "id": 42,
  "method": "drying",
  "params": {
    "temp": 50,
    "fan_speed": 7000,
    "duration": 240
  }
}
```

**Full V1 `drying` response (from ACE Pro hardware):**
```json
{
  "code": 0,
  "id": 42,
  "msg": "success",
  "result": {}
}
```

**V1 `drying_stop` request:**
```json
{ "id": 43, "method": "drying_stop" }
```

**V1 `start_feed_assist` response (confirming feed assist enabled):**
```json
{
  "code": 0,
  "id": 1,
  "msg": "success",
  "result": {}
}
```

#### 3.6.0.1 UDS-to-Serial Command Flow (Dryer Example)

The complete flow when a user presses the "Dryer" switch on the touchscreen for a V1 ACE:

```
UI (K3SysUi)
    |
    |  JSON-RPC over UDS (/tmp/unix_uds1):
    |  {"method": "filament_hub/start_drying", "params": {"duration": 240, "target_temp": 50}, "id": 85}
    v
Webhooks.go (endpoint handler)
    |
    |  Calls ace.Cmd_ACE_START_DRYING() which calls:
    |  self.send_request({"method": "drying", "params": {"temp": 50, "fan_speed": 7000, "duration": 240}}, callback)
    v
AceCommun.Writer() (queue pump)
    |
    |  Assigns id=42, registers callback in callback_map[42]
    |  Calls self._send_request({"id": 42, "method": "drying", "params": {...}})
    v
_send_request() (extras_ace_commun_proxy.go)
    |
    |  Checks self.IsV2:
    |    - V1 (IsV2=false): return self._send_request_v1(req)  --> frames as JSON, writes to serial
    |    - V2 (IsV2=true):  serializes DryingRequest protobuf, writes raw frame to serial FD
    v
ACE Hardware (/dev/ttyACM0)
    |
    |  V1 response: {"code": 0, "id": 42, "msg": "success", "result": {}}
    v
AceCommun.Reader()
    |
    |  Validates frame (start bytes, length, CRC, end byte)
    |  IsV2=false: skips protobuf parse, JSON unmarshals payload
    |  Extracts id=42, finds callback_map[42], invokes callback
    v
Callback in extras_ace.go
    |
    |  Updates self.info dryer status
    |  Next heartbeat poll confirms dryer_status: {"status": "drying", "target_temp": 50, ...}
    v
Subscription diffing (QueryStatusHelper)
    |
    |  Detects dryer status change, sends:
    |  {"method": "notify_status_update", "params": {"status": {"filament_hub": {"filament_hubs": [...]}}}}
    v
UI updates dryer switch to "active"
```

#### 3.6.1 Protobuf Definitions

Source: `k3c/filament/ace_v2/ace_proto_v2.proto`

```protobuf
syntax = "proto3";
package ace_v2;

message SetFanRequest {
    uint32 speed = 1;
}

message SetRfidEnableRequest {
    bool enable = 1;
}

message DryingRequest {
    uint32 temperature = 1;
    uint32 duration_hours = 2;
}

message FeedOrRollbackRequest {
    int32 length = 1;    // positive = feed, negative = rollback
    uint32 speed = 2;
}

message FilamentInfoResponse {
    uint32 slot_index = 1;
    string color_hex = 2;
    uint32 current_temp = 3;
    bool is_drying = 4;
}

message StatusResponse {
    uint32 state = 1;        // 0=unknown, 2=feeding, other=ready
    uint32 current_temp = 2;
    uint32 target_temp = 3;  // >0 means dryer is active
    bool error_flag = 4;
}
```

#### 3.6.2 Command Mapping (JSON method to V2 protobuf)

The request dispatch in `extras_ace_commun_proxy.go` intercepts outbound JSON requests and converts them to V2 protobuf. **This switch is only entered when `IsV2 == true`.** When `IsV2 == false`, all commands bypass the proxy and are sent as native V1 JSON via `_send_request_v1()`.

| JSON `method`/`cmd` | Protobuf Message | Details |
|---|---|---|
| `"feed"` | `FeedOrRollbackRequest{Length: +50, Speed: 50}` | Fixed 50mm forward |
| `"rollback"` | `FeedOrRollbackRequest{Length: -50, Speed: 50}` | Fixed 50mm backward |
| `"set_rfid"` | `SetRfidEnableRequest{Enable: bool}` | From `req["enable"]` |
| `"drying"` | `DryingRequest{Temperature: T, DurationHours: H}` | Temp from `params["temp"]`, duration from `params["duration"]` converted minutes-to-hours |
| `"drying_stop"` | `DryingRequest{Temperature: 0, DurationHours: 0}` | Zero values to stop |
| (unmatched) | JSON fallback | Falls through to `_send_request_v1(req)` |

**V2 dryer callback synthesis:** When a V2 protobuf frame is successfully written (e.g., for `drying`), the proxy immediately synthesizes a success callback for the pending UDS request. This prevents the UI from timing out and visually reverting the dryer toggle:

```go
if idVal, ok := req["id"]; ok {
    if id, ok := idVal.(int); ok {
        if cb, ok := self.callback_map[id]; ok {
            delete(self.callback_map, id)
            cb(map[string]interface{}{
                "id":     float64(id),
                "result": map[string]interface{}{"success": true},
            })
        }
    }
}
```

#### 3.6.3 V2 Response Handling

`Reader()` in `extras_ace_commun.go` conditionally parses V2 responses based on the `IsV2` flag:

```
1. Validate frame (start bytes, end byte, length, CRC via V1 _calc_crc)
2. Extract payload bytes
3. If IsV2:
   a. Try V2: filament.NewV2ProtoHandler().ParseResponse(payload)
      - ParseResponse re-validates the frame with V2 Modbus CRC
      - On success: TranslateV2ResponseToJSON(unwrappedPayload)
   b. Assign pseudo-ID from callback_map
4. If ret is still nil (V1 mode, or V2 parse failed): json.Unmarshal(payload)
5. Extract response ID (type-switch handles float64 from V1 JSON and int from V2 pseudo-ID)
6. Match response to callback by ID, invoke and delete
```

**Critical design note:** When `IsV2 == false`, the V2 protobuf parser is **never invoked**. This is essential because the V2 `ParseResponse` can misinterpret valid V1 JSON bytes as protobuf data, silently consuming the response and producing garbage. This was the root cause of a connection timeout loop where `get_status` responses were being eaten by the V2 parser on V1 hardware, causing the ACE to disconnect and reconnect every ~3 seconds.

#### 3.6.4 V2-to-JSON Translation

`TranslateV2ResponseToJSON()` converts a `StatusResponse` protobuf into a V1-compatible JSON map:

```go
// Input: raw protobuf bytes from StatusResponse
// Output: V1-compatible map for the rest of the pipeline

{
  "result": {
    "status": "ready",           // "feeding" if State == 2
    "temp":   currentTemp,
    "dryer": {
      "status":      "drying",   // "stop" if TargetTemp == 0
      "target_temp": targetTemp,
      "duration":    0,          // not available in V2
      "remain_time": 0,          // not available in V2
    },
    "fan_speed": 7000,           // hardcoded default
    "slots": [                   // 4 minimal slots, all "ready"
      {"index": 0, "status": "ready"},
      {"index": 1, "status": "ready"},
      {"index": 2, "status": "ready"},
      {"index": 3, "status": "ready"},
    ]
  }
}
```

Note: V2 `StatusResponse` does not carry per-slot filament data (type, color, sku). The custom slot merging in Phase 3 (Section 5) fills those in from `self.custom_slots`.

#### 3.6.5 Serialization Helpers

Located in `k3c/filament/ace_v2_proxy.go`:

| Function | Input | Output |
|---|---|---|
| `SerializeFeedOrRollback(isFeed bool, speed int)` | Direction + speed | Protobuf bytes (length=+100 or -100) |
| `SerializeSetFan(speed int)` | Fan speed 0-65535 | Protobuf bytes |
| `SerializeDrying(enable bool, time int)` | Enable + duration (minutes, converted to hours) | Protobuf bytes (temp=50 if enabled, else 0) |
| `SerializeSetRfidEnable(enable bool)` | Enable flag | Protobuf bytes |
| `SerializeSetFeedCheck(enable bool)` | Enable flag | Protobuf bytes (reuses SetRfidEnableRequest) |
| `SerializeAssignDeviceId()` | (none) | Empty `[]byte{}` |
| `ParseStatusResponse(data []byte)` | Raw protobuf bytes | `*ace_v2.StatusResponse` |
| `ParseFilamentInfoResponse(data []byte)` | Raw protobuf bytes | `*ace_v2.FilamentInfoResponse` |

### 3.7 Request Queue

`AceCommun` uses a thread-safe FIFO queue (`container/list.List` with `sync.Mutex`):
1. `Push_send_queue(request_map, callback_func)` wraps into a `RequestInfo` struct and enqueues.
2. `Writer()` dequeues, assigns an auto-incremented `id`, registers the callback in `callback_map[id]`, frames the JSON, and writes to serial.
3. `Reader()` parses incoming frames, validates CRC, JSON-decodes the payload, extracts the `id`, looks up and invokes `callback_map[id]`, then deletes the entry.

### 3.8 Reconnection

- Max attempts: 10 (`RECONNECT_COUNT`)
- Delay between attempts: `0.8 * attempt + cos(attempt) * 0.5`
- Timeout: cumulative delay exceeding 10 seconds
- On reconnect: re-enables feed assist if it was active before disconnect

---

## 4. Data Model (In-Memory State)

### 4.1 `self.custom_slots` (per-slot user-defined metadata)

Initialized as a 4-element slice of maps:

```go
custom_slots = []map[string]interface{}{
    {"type": "", "color": []interface{}{0.0, 0.0, 0.0}},
    {"type": "", "color": []interface{}{0.0, 0.0, 0.0}},
    {"type": "", "color": []interface{}{0.0, 0.0, 0.0}},
    {"type": "", "color": []interface{}{0.0, 0.0, 0.0}},
}
```

On boot, populated from `ams_config.cfg` entries. Keys per slot: `type` (string), `color` (`[]interface{}` of float64), `colors` (`[]interface{}` if present in config).

### 4.2 `self.info` (full ACE status, merged with hardware data)

Default initialization:

```json
{
  "status": "ready",
  "dryer": {
    "status": "stop",
    "target_temp": 0,
    "duration": 0,
    "remain_time": 0
  },
  "temp": 0,
  "enable_rfid": 1,
  "fan_speed": 7000,
  "feed_assist_count": 0,
  "cont_assist_time": 0.0,
  "slots": [
    {
      "index": 0,
      "status": "ready",
      "sku": "",
      "type": "<from custom_slots[0]>",
      "color": "<from custom_slots[0]>",
      "colors": "<from custom_slots[0]>",
      "icon_type": 0,
      "remain": 0,
      "decorder": 0,
      "rfid": 1,
      "source": 2
    }
  ]
}
```

Slots with no configured data (empty `type` from `custom_slots`) are initialized with `"status": "empty"` instead of `"ready"`.

### 4.3 `self.inventory` (persistent slot inventory)

```go
[]map[string]interface{}{
    {"status": "empty", "color": []interface{}{0, 0, 0}, "material": "", "temp": 0},
    // ... 4 slots
}
```

Persisted via `SAVE_VARIABLE VARIABLE=ace_inventory VALUE=<json>`.

### 4.4 Persistent Variables (via SaveVariables)

| Variable | Type | Description |
|---|---|---|
| `ace_current_index` | int64 | Currently loaded tool index (-1 = none) |
| `ace_filament_pos` | string | Position: `"bowden"`, `"spliter"`, `"toolhead"`, `"nozzle"` |
| `ace_endless_spool_enabled` | bool | Endless spool feature flag |
| `ace_inventory` | JSON array | Saved slot inventory |

### 4.5 `Get_status()` Output (what the subscription system sees)

`Get_status(eventtime)` wraps `self.info` into a larger envelope for the Klipper status subscription system:

```json
{
  "auto_refill": 0,
  "current_filament": "",
  "cutter_state": 0,
  "ext_spool": 1,
  "ext_spool_status": "runout",
  "filament_hubs": [ "<deep copy of self.info with id, endless_spool, fw_info added>" ],
  "filament_present": 0,
  "tracker_detection_length": 0,
  "tracker_filament_present": 0
}
```

The `filament_hubs` array contains a single element: a `sys.DeepCopyMap()` of the status map, which includes `self.info` plus `id`, `endless_spool`, and `fw_info`.

---

## 5. End-to-End Implementation Flow

### Phase 1: Boot Initialization

1. `NewAce(config)` reads `[filament_hub]` config section for serial port, baud rate, movement params.
2. Reads `/userdata/app/gk/config/ams_config.cfg`, populates `self.custom_slots[0..3]` with `type`, `color`, `colors` from each filament entry.
3. Constructs `self.info` with the default structure above, slotting in `custom_slots` data. Slots with non-empty type get `"status": "ready"`; empty slots get `"status": "empty"`.
4. Loads persistent variables (`ace_current_index`, `ace_filament_pos`, `ace_endless_spool_enabled`, `ace_inventory`).
5. Registers GCode commands, webhook endpoints, and timers.

### Phase 2: Connection & Heartbeat

1. `_connect()` opens the serial device with termios settings, registers FD with the reactor event loop.
2. On successful connection, activates `heatbeat_timer` (fires immediately, then every **1.5 seconds**).
3. Each heartbeat sends `{"method": "get_status"}` to the ACE hardware via serial.
4. The response callback merges hardware status with `custom_slots` data (see Phase 3).

### Phase 3: Status Merging (Custom Slots into Hardware Response)

When the heartbeat callback receives a `get_status` response from hardware:

1. Iterate over `res["slots"]` (the hardware-reported slot array).
2. For each slot where `sku` is empty, nil, or `"custom"`:
   - Override with spoof fields: `status="ready"`, `sku=""`, `rfid=1`, `source=2`, `icon_type=0`, `remain=0`, `decorder=0`
   - Overlay `type`, `color`, `colors` from `self.custom_slots[i]`
   - If no `colors` exists in custom_slots, default to `[[0, 0, 0, 0]]`
3. Store the merged result as `self.info`.

This ensures that every poll cycle, the in-memory state reflects hardware telemetry with custom metadata overlaid.

### Phase 4: Request Interception & Write-Back

When the user taps a custom slot in the UI:

1. UI sends JSON-RPC to `filament_hub/set_filament_info` with `{index, type, color: {R, G, B}}`.
2. `Webhooks.go` handler extracts params, converts `color` map to `[]interface{}{R, G, B}`.
3. Calls `ace.Set_filament_info(index, type, "", color)` via the `FilamentInfoSetter` interface.
4. `Set_filament_info()`:
   - Updates `self.custom_slots[index]` with new type, color, and computed colors `[[R, G, B, 255]]`.
   - Deep-copies `self.info` (via JSON marshal/unmarshal), updates the target slot with all spoof fields, replaces `self.info`.
   - Reads `ams_config.cfg` from disk, updates the slot entry, writes it back (mode `0644`).

### Phase 5: Subscription Diffing & Reactive Broadcast

1. The UI subscribes to status updates via the `objects/subscribe` endpoint, specifying which object keys to watch (e.g., `filament_hub`).
2. `QueryStatusHelper` runs a timer every **250ms** (`SUBSCRIPTION_REFRESH_TIME`).
3. For each subscribed client, it calls `Get_status(eventtime)` on each watched object.
4. For each requested field, it compares the current value against `last_query` using `reflect.DeepEqual()`.
5. Only changed fields are included in the outbound message. If no fields changed, no message is sent.
6. The response is sent as:
   ```json
   { "method": "notify_status_update", "params": { "eventtime": 1234.5, "status": { "filament_hub": { ...changed_fields... } } } }
   ```
7. `last_query` is updated with the current query results for the next diff cycle.

---

## 6. GCode Commands

| Command | Parameters | Description |
|---|---|---|
| `ACE_SET_SLOT` | `INDEX=0-3 [COLOR=R,G,B] [MATERIAL=name] [TEMP=200-260] [EMPTY=1]` | Set or clear a slot in inventory |
| `ACE_QUERY_SLOTS` | (none) | Print current slot inventory |
| `ACE_DEBUG` | `METHOD=name [PARAMS=json]` | Send arbitrary method to ACE serial |
| `ACE_START_DRYING` | `TEMP=max55 DURATION=minutes` | Start dryer |
| `ACE_STOP_DRYING` | (none) | Stop dryer |
| `ACE_ENABLE_FEED_ASSIST` | `INDEX=0-3` | Enable feed assist on slot |
| `ACE_DISABLE_FEED_ASSIST` | `[INDEX=0-3]` | Disable feed assist (default: current) |
| `ACE_FEED` | `INDEX=0-3 LENGTH=mm [SPEED=50]` | Feed filament forward |
| `ACE_RETRACT` | `INDEX=0-3 LENGTH=mm [SPEED=50]` | Retract filament |
| `ACE_CHANGE_TOOL` | `TOOL=0-3` | Full toolchange sequence |
| `ACE_ENABLE_ENDLESS_SPOOL` | (none) | Enable endless spool monitoring |
| `ACE_DISABLE_ENDLESS_SPOOL` | (none) | Disable endless spool monitoring |
| `ACE_ENDLESS_SPOOL_STATUS` | (none) | Print endless spool state |
| `ACE_SAVE_INVENTORY` | (none) | Persist inventory to variables |
| `ACE_TEST_RUNOUT_SENSOR` | (none) | Test runout sensor state |

---

## 7. Toolchange Sequence (`ACE_CHANGE_TOOL`)

1. Disable feed assist on the currently loaded tool.
2. Retract filament back to ACE (`toolchange_retract_length` mm, default 150).
3. Feed filament from the new slot (`toolchange_load_length` mm, default 630).
4. Wait for extruder sensor activation.
5. Enable feed assist on the new tool.
6. Move extruder forward to nozzle (`toolhead_sensor_to_nozzle_length` mm, default 0).
7. Save `ace_current_index` and `ace_filament_pos` to persistent variables.

**Configuration defaults** (from `[filament_hub]` config section):
- `feed_speed`: 50
- `retract_speed`: 50
- `toolchange_retract_length`: 150 mm
- `toolchange_load_length`: 630 mm
- `toolhead_sensor_to_nozzle_length`: 0 mm
- `max_dryer_temperature`: 55 C

---

## 8. Endless Spool Monitoring

When enabled, a monitor timer polls the extruder runout sensor:
- **Poll rate**: every 50ms during printing, 200ms when idle.
- On runout detection:
  1. Find the next available (non-empty) slot.
  2. Disable feed assist on the current tool.
  3. Feed from the next slot until the extruder sensor triggers.
  4. Enable feed assist on the new slot.
  5. Update persistent variables.
  6. Continue printing.

---

## 9. Configuration Reference (`[filament_hub]` section)

| Key | Default | Description |
|---|---|---|
| `serial` | `/dev/ttyACM0` | Serial device path (auto-detect via `/dev/ttyACM*`) |
| `v2_baud` | `230400` | Baud rate (also supports 115200) |
| `feed_speed` | `50` | Feed speed (mm/s) |
| `retract_speed` | `50` | Retract speed (mm/s) |
| `toolchange_retract_length` | `150` | Retract distance for toolchange (mm) |
| `toolchange_load_length` | `630` | Load distance for toolchange (mm) |
| `toolhead_sensor_to_nozzle_length` | `0` | Distance from sensor to nozzle (mm) |
| `max_dryer_temperature` | `55` | Maximum dryer temperature (C) |
| `endless_spool` | `false` | Enable endless spool at startup |

---

## 10. Known Bugs & Design Constraints

### 10.1 The Slices Deep-Copy Bug (`common/utils/sys/SysUtils.go`)

**Symptom:** Custom filaments would correctly route to the cache, but the UI refused to update screen visuals.

**Root Cause:** `sys.DeepCopyMap()` historically failed to deep-copy nested slices (`[]interface{}`). Because `slots` is a nested array, dynamically altering properties on the array mutated the previous cached tick history simultaneously via Go pointer aliasing.

**Effect:** `reflect.DeepEqual()` in the subscription diffing engine compared identical aliased maps and continuously returned zero changes, suppressing all outbound status updates to the touchscreen.

**Resolution:** `DeepCopy()` now handles three cases recursively:
- `map[string]interface{}` -- creates new map, recursively copies values
- `[]interface{}` -- `make([]interface{}, len(s))` and recursively copies each element
- Primitives -- returned as-is

This is critical: `Get_status()` calls `sys.DeepCopyMap(status)` before returning, ensuring the subscription system always compares independent snapshots.

### 10.2 Slot Status Must Be "ready" for Custom Filaments

The UI will not render a slot whose `status` is `"empty"`, even if all other metadata (type, color) is present. Custom slots must always be initialized and merged with `"status": "ready"`.

### 10.3 Missing Spoof Fields Cause Silent Packet Drops

The UI silently ignores slot data missing any of: `icon_type`, `remain`, `decorder`, `rfid`, `source`. All must be explicitly present.

### 10.4 V2 Response ID Matching is Fragile

V2 protobuf responses carry no `id` field. The code assigns an ID by iterating `callback_map` and grabbing the first key (`for pendingId := range self.callback_map { ... break }`). This relies on Go map iteration order, which is intentionally randomized. It works in practice only because the queue is serial -- there is at most one outstanding request at a time. If request pipelining is ever added, this must be replaced with a proper FIFO tracking structure.

### 10.5 Dual CRC Implementations

The V1 reader (`extras_ace_commun.go`) and V2 handler (`ace_v2_proxy.go`) use **different CRC algorithms** (see Section 3.4). Both validate the same frame format. This means the V1 `Reader()` validates incoming frames with the V1 CRC over payload only, then hands the payload to the V2 handler which re-validates using Modbus CRC over the full frame prefix + payload. If the hardware uses one CRC scheme, the other validation may silently pass or fail depending on data. This dual validation is a known artifact of the incremental V1-to-V2 migration.

### 10.6 V2 Status Lacks Per-Slot Detail

The V2 `StatusResponse` protobuf only carries global state (device state, current/target temp, error flag). It does **not** include per-slot filament metadata (type, color, sku). The `TranslateV2ResponseToJSON()` function fills in 4 minimal stub slots with `"status": "ready"` and no other data. All actual per-slot filament info comes from `self.custom_slots` during the merging phase. This means with V2, the backend is fully responsible for slot metadata -- nothing comes from hardware.

### 10.7 V2 Protobuf Parser Must Not Run on V1 Responses (RESOLVED)

**Symptom:** ACE Pro (V1) connected successfully, including `get_info` and `start_feed_assist` responses, but `get_status` responses were never processed. The ACE would timeout after ~3 seconds and enter an infinite reconnect loop (connect -> get_info OK -> get_status timeout -> disconnect -> reconnect).

**Root Cause:** The V2 `ParseResponse()` was being called unconditionally on every incoming frame (before the `IsV2` gating was added). For V1 JSON responses, `ParseResponse()` would sometimes not return an error but would produce garbage data that was then passed to `TranslateV2ResponseToJSON()`. The resulting `ret` map had no valid `id`, causing the callback lookup to fail silently. Meanwhile, the original JSON response was consumed and never reached the `json.Unmarshal` fallback.

**Resolution:** The V2 parse path is now gated behind `if self.IsV2 { ... }`. When `IsV2 == false`, only `json.Unmarshal` is used. The `IsV2` flag is set during the `get_info` handshake based on the hardware model string.

**Why `get_info` worked but `get_status` didn't:** The `get_info` JSON response happened to fail the V2 parse cleanly (errV2 != nil), so it fell through to JSON unmarshal. The `get_status` response, being larger and structurally different, happened to partially pass the V2 parse, producing a non-nil but invalid result that blocked the JSON fallback.

### 10.8 Missing Heartbeat Timer Activation Causes Silent Polling Failure (RESOLVED)

**Symptom:** After code edits, the ACE would connect and get firmware info successfully, but `get_status` polling never started. The ACE appeared "connected" but was non-functional (no dryer, no slot updates).

**Root Cause:** The `heatbeat_timer` update (`self.reactor.Update_timer(self.heatbeat_timer, constants.NOW)`) was accidentally removed during a patch to the `_connect()` function. Without this call, the timer remained in its initial `NEVER` state and `_periodic_heartbeat_event` was never invoked.

**Resolution:** Restored the timer activation after the `get_info` request in `_connect()`.

---

## 11. File Map

| File | Responsibility |
|---|---|
| `project/extras_ace.go` | ACE struct, state management, merging, GCode commands, toolchange, endless spool, **V1/V2 model detection in `_connect()` via `get_info` response** |
| `project/extras_ace_commun.go` | Serial communication: binary framing, V1 CRC, request queue, reader/writer, termios, **`IsV2` flag**, **conditional V2 parsing in `Reader()`** |
| `project/extras_ace_commun_proxy.go` | V2 request dispatch (`_send_request`): **V1 bypass when `!IsV2`**, intercepts JSON commands for V2, serializes to protobuf. Also `TranslateV2ResponseToJSON()`, **UDS callback synthesis for V2 commands** |
| `k3c/filament/ace_v2_proxy.go` | V2 frame handler (`V2ProtoHandler`): `BuildRequest`, `ParseResponse`, Modbus CRC. All protobuf serialize/parse helpers |
| `k3c/filament/ace_v2/ace_proto_v2.proto` | Protobuf message definitions (source of truth for V2 wire format) |
| `k3c/filament/ace_v2/ace_proto_v2.pb.go` | Generated Go code from the proto file |
| `project/Webhooks.go` | UDS server, JSON-RPC routing, endpoint handlers, subscription diffing (`QueryStatusHelper`) |
| `common/utils/sys/SysUtils.go` | `DeepCopy()` and `DeepCopyMap()` utilities |
| `project/queue/Queue.go` | Thread-safe FIFO queue (used by `AceCommun` request queue) |

---

## 12. Constants

```go
// Serial frame markers
FRAME_START_1  = 0xFF
FRAME_START_2  = 0xAA
FRAME_END      = 0xFE
MIN_FRAME_SIZE = 7

// Subscription refresh
SUBSCRIPTION_REFRESH_TIME = 0.25  // 250ms

// Error strings
RESPOND_TIMEOUT_ERROR  = "Respond timeout with the ACE PRO"
UNABLE_TO_COMMUN_ERROR = "Unable to communicate with the ACE PRO"
OPEN_REMOTE_DEV_ERROR  = "Unable to open remote dev"
OPEN_SERIAL_DEV_ERROR  = "Unable to open serial port"
NOT_FOUND_SERIAL_ERROR = "Not found serial port"

// Reconnection
RECONNECT_COUNT = 10
```
