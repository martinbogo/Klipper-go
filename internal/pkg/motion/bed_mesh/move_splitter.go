package bedmesh

import "math"

type MoveSplitter struct {
	Split_delta_z       float64
	Move_check_distance float64
	Z_mesh              *ZMesh
	Fade_offset         float64
	Pre_pos             []float64
	Next_pos            []float64
	Current_pos         []float64
	Z_factor            float64
	Z_offset            float64
	Traverse_complete   bool
	Distance_checked    float64
	Total_move_length   float64
	Axis_move           []bool
}

func NewMoveSplitter(splitDeltaZ float64, moveCheckDistance float64) *MoveSplitter {
	return &MoveSplitter{
		Split_delta_z:       splitDeltaZ,
		Move_check_distance: moveCheckDistance,
	}
}

func (self *MoveSplitter) Initialize(mesh *ZMesh, fade_offset float64) {
	self.Z_mesh = mesh
	self.Fade_offset = fade_offset
}

func (self *MoveSplitter) Build_move(prev_pos []float64, next_pos []float64, factor float64) {
	self.Pre_pos = append([]float64{}, prev_pos...)
	self.Next_pos = append([]float64{}, next_pos...)
	self.Current_pos = append([]float64{}, prev_pos...)
	self.Z_factor = factor
	self.Z_offset = self.Calc_z_offset(prev_pos)
	self.Traverse_complete = false
	self.Distance_checked = 0.
	axes_d := []float64{}
	for i := 0; i < 4; i++ {
		axes_d = append(axes_d, self.Next_pos[i]-self.Pre_pos[i])
	}
	sum_val := 0.
	for i := 0; i < 3; i++ {
		sum_val += axes_d[i] * axes_d[i]
	}
	self.Total_move_length = math.Sqrt(sum_val)
	var axis_move []bool
	for _, d := range axes_d {
		axis_move = append(axis_move, !Isclose(d, 0, 1e-10, 0))
	}
	self.Axis_move = axis_move
}

func (self *MoveSplitter) Calc_z_offset(pos []float64) float64 {
	z := self.Z_mesh.Calc_z(pos[0], pos[1])
	offset := self.Fade_offset
	return self.Z_factor*(z-offset) + offset
}

func (self *MoveSplitter) Set_next_move(distance_from_prev float64) error {
	t := distance_from_prev / self.Total_move_length
	if t > 1. || t < 0 {
		panic("bed_mesh: Slice distance is negative or greater than entire move length")
	}
	for i := 0; i < 4; i++ {
		if self.Axis_move[i] {
			self.Current_pos[i] = Lerp(t, self.Pre_pos[i], self.Next_pos[i])
		}
	}
	return nil
}

func (self *MoveSplitter) Split() []float64 {
	if !self.Traverse_complete {
		if self.Axis_move[0] || self.Axis_move[1] {
			for self.Distance_checked+self.Move_check_distance < self.Total_move_length {
				self.Distance_checked += self.Move_check_distance
				self.Set_next_move(self.Distance_checked)
				next_z := self.Calc_z_offset(self.Current_pos)
				if math.Abs(next_z-self.Z_offset) >= self.Split_delta_z {
					self.Z_offset = next_z
					sum_val := self.Current_pos[2] + self.Z_offset
					return []float64{self.Current_pos[0], self.Current_pos[1], sum_val, self.Current_pos[3]}
				}
			}
		}
		self.Current_pos = append([]float64{}, self.Next_pos...)
		self.Z_offset = self.Calc_z_offset(self.Current_pos)
		self.Current_pos[2] += self.Z_offset
		self.Traverse_complete = true
		return self.Current_pos
	}
	return nil
}
