package bedmesh

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// Vec2 is a 2-dimensional point with X and Y coordinates.
type Vec2 struct {
	X float64
	Y float64
}

// MoveState holds the current head position while parsing a G-code file.
type MoveState struct {
	X float64
	Y float64
	Z float64
}

// MoveCommand holds the parsed parameters of a single G-code move.
type MoveCommand struct {
	X    float64
	Y    float64
	Z    float64
	E    float64
	F    float64
	HasX bool
	HasY bool
	HasZ bool
	HasE bool
	HasF bool
}

// ParseVec2CSV parses two comma-separated float values into a Vec2.
func ParseVec2CSV(input string) (Vec2, error) {
	parts := strings.Split(input, ",")
	if len(parts) != 2 {
		return Vec2{}, fmt.Errorf("expected two comma separated values")
	}
	var values [2]float64
	for i := range parts {
		val, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
		if err != nil {
			return Vec2{}, err
		}
		values[i] = val
	}
	return Vec2{X: values[0], Y: values[1]}, nil
}

// GetPolygonMinMax returns the bounding box of a set of 2D points.
func GetPolygonMinMax(points []Vec2) (Vec2, Vec2, error) {
	if len(points) == 0 {
		return Vec2{}, Vec2{}, fmt.Errorf("no points provided")
	}
	minX, maxX := math.Inf(1), math.Inf(-1)
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, pt := range points {
		if pt.X < minX {
			minX = pt.X
		}
		if pt.X > maxX {
			maxX = pt.X
		}
		if pt.Y < minY {
			minY = pt.Y
		}
		if pt.Y > maxY {
			maxY = pt.Y
		}
	}
	return Vec2{X: minX, Y: minY}, Vec2{X: maxX, Y: maxY}, nil
}

// GetMoveMinMax returns the bounding box of a list of vertices.
func GetMoveMinMax(vertices []Vec2) (Vec2, Vec2, error) {
	if len(vertices) == 0 {
		return Vec2{}, Vec2{}, fmt.Errorf("no vertices provided")
	}
	minX, maxX := math.Inf(1), math.Inf(-1)
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, v := range vertices {
		if v.X < minX {
			minX = v.X
		}
		if v.X > maxX {
			maxX = v.X
		}
		if v.Y < minY {
			minY = v.Y
		}
		if v.Y > maxY {
			maxY = v.Y
		}
	}
	return Vec2{X: minX, Y: minY}, Vec2{X: maxX, Y: maxY}, nil
}

// GetLayerMinMaxBeforeFade returns the bounding box of all extrude layers below fadeEnd.
// If fadeEnd is 0, all layers are included.
func GetLayerMinMaxBeforeFade(layers map[float64][]Vec2, fadeEnd float64) (Vec2, Vec2, error) {
	fadeLimit := fadeEnd
	if fadeLimit == 0 {
		fadeLimit = math.Inf(1)
	}

	points := make([]Vec2, 0, len(layers)*2)
	for height, vertices := range layers {
		if height >= fadeLimit {
			continue
		}
		if len(vertices) == 0 {
			continue
		}
		minPt, maxPt, err := GetMoveMinMax(vertices)
		if err != nil {
			return Vec2{}, Vec2{}, err
		}
		points = append(points, minPt, maxPt)
	}

	if len(points) == 0 {
		return Vec2{}, Vec2{}, fmt.Errorf("no printable layers within fade window")
	}

	return GetPolygonMinMax(points)
}

// ParseLinearMove parses a G0/G1 move token list into a MoveCommand.
func ParseLinearMove(tokens []string) (MoveCommand, error) {
	var mv MoveCommand
	for _, token := range tokens[1:] {
		if len(token) < 2 {
			continue
		}
		axis := strings.ToUpper(token[:1])
		val, err := strconv.ParseFloat(token[1:], 64)
		if err != nil {
			return MoveCommand{}, err
		}
		switch axis {
		case "X":
			mv.X = val
			mv.HasX = true
		case "Y":
			mv.Y = val
			mv.HasY = true
		case "Z":
			mv.Z = val
			mv.HasZ = true
		case "E":
			mv.E = val
			mv.HasE = true
		case "F":
			mv.F = val
			mv.HasF = true
		}
	}
	return mv, nil
}

// ParseArcMoves parses G2/G3 arc move tokens into a sequence of linear MoveCommands.
// arcSegments controls the number of line segments used to approximate the arc.
func ParseArcMoves(cmd string, tokens []string, state MoveState, arcSegments int) ([]MoveCommand, error) {
	params := make(map[string]float64)
	flags := make(map[string]bool)
	for _, token := range tokens[1:] {
		if len(token) < 2 {
			continue
		}
		axis := strings.ToUpper(token[:1])
		val, err := strconv.ParseFloat(token[1:], 64)
		if err != nil {
			return nil, err
		}
		params[axis] = val
		flags[axis] = true
	}

	start := Vec2{X: state.X, Y: state.Y}
	end := start
	if flags["X"] {
		end.X = params["X"]
	}
	if flags["Y"] {
		end.Y = params["Y"]
	}

	center := Vec2{X: start.X + params["I"], Y: start.Y + params["J"]}

	radius := math.Hypot(params["I"], params["J"])
	if flags["R"] {
		radius = params["R"]
	}
	if radius == 0 {
		return nil, fmt.Errorf("invalid arc radius")
	}

	startAngle := math.Atan2(start.Y-center.Y, start.X-center.X)
	endAngle := math.Atan2(end.Y-center.Y, end.X-center.X)
	angleDelta := endAngle - startAngle
	if cmd == "G3" {
		if angleDelta < 0 {
			angleDelta += 2 * math.Pi
		}
	} else if cmd == "G2" {
		if angleDelta > 0 {
			angleDelta -= 2 * math.Pi
		}
	}

	segments := arcSegments
	if segments <= 0 {
		segments = 1
	}
	angleIncrement := angleDelta / float64(segments)
	moves := make([]MoveCommand, 0, segments+1)

	for i := 0; i <= segments; i++ {
		angle := startAngle + float64(i)*angleIncrement
		x := center.X + radius*math.Cos(angle)
		y := center.Y + radius*math.Sin(angle)
		mv := MoveCommand{X: x, Y: y, HasX: true, HasY: true}
		if flags["Z"] {
			mv.Z = params["Z"]
			mv.HasZ = true
		}
		if flags["E"] {
			mv.E = params["E"]
			mv.HasE = true
		}
		if flags["F"] {
			mv.F = params["F"]
			mv.HasF = true
		}
		moves = append(moves, mv)
	}
	return moves, nil
}

// DecodeMoves dispatches a G-code command to the appropriate move parser.
// Returns nil, nil for unrecognised commands.
func DecodeMoves(cmd string, tokens []string, state MoveState, arcSegments int) ([]MoveCommand, error) {
	switch cmd {
	case "G0", "G1":
		mv, err := ParseLinearMove(tokens)
		if err != nil {
			return nil, err
		}
		return []MoveCommand{mv}, nil
	case "G2", "G3":
		return ParseArcMoves(cmd, tokens, state, arcSegments)
	default:
		return nil, nil
	}
}

// ApplyMove applies a MoveCommand to the current MoveState and returns the new state.
// absolute controls whether coordinates are absolute or relative.
func ApplyMove(state MoveState, move MoveCommand, absolute bool) MoveState {
	newState := state
	if move.HasX {
		if absolute {
			newState.X = move.X
		} else {
			newState.X += move.X
		}
	}
	if move.HasY {
		if absolute {
			newState.Y = move.Y
		} else {
			newState.Y += move.Y
		}
	}
	if move.HasZ {
		if absolute {
			newState.Z = move.Z
		} else {
			newState.Z += move.Z
		}
	}
	return newState
}

// GetLayerVertices parses a G-code file and returns extrusion vertex positions grouped by Z height.
// arcSegments controls arc approximation precision.
func GetLayerVertices(filePath string, arcSegments int) (map[float64][]Vec2, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	state := MoveState{}
	absoluteMove := true
	extrudeLayers := make(map[float64][]Vec2)

	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, ";"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		tokens := strings.Fields(line)
		if len(tokens) == 0 {
			continue
		}
		cmd := strings.ToUpper(tokens[0])

		switch cmd {
		case "G90":
			absoluteMove = true
			continue
		case "G91":
			absoluteMove = false
			continue
		}

		moves, err := DecodeMoves(cmd, tokens, state, arcSegments)
		if err != nil {
			return nil, err
		}
		if len(moves) == 0 {
			continue
		}

		for _, mv := range moves {
			newState := ApplyMove(state, mv, absoluteMove)

			if !(mv.HasX || mv.HasY || mv.HasZ) {
				state = newState
				continue
			}

			if newState.Z == 0 {
				state = newState
				continue
			}

			if mv.HasE && mv.E > 0 {
				extrudeLayers[newState.Z] = append(extrudeLayers[newState.Z], Vec2{X: newState.X, Y: newState.Y})
			}

			state = newState
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	for z, vertices := range extrudeLayers {
		if len(vertices) == 0 {
			delete(extrudeLayers, z)
		}
	}

	if len(extrudeLayers) == 0 {
		return nil, fmt.Errorf("no extrude layers detected in gcode")
	}

	return extrudeLayers, nil
}
