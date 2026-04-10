package bedmesh

import (
	"fmt"
	"goklipper/common/logger"
	"math"
	"strings"
)

type ZMesh struct {
	Probed_matrix [][]float64
	Mesh_matrix   [][]float64
	Mesh_params   map[string]interface{}
	Avg_z         float64
	Mesh_offsets  []float64
	Mesh_x_min    float64
	Mesh_x_max    float64
	Mesh_y_min    float64
	Mesh_y_max    float64
	Mesh_x_count  int
	Mesh_y_count  int
	X_mult        int
	Y_mult        int
	Mesh_x_dist   float64
	Mesh_y_dist   float64
	Sample        func([][]float64)
}

func NewZMesh(params map[string]interface{}) *ZMesh {
	self := &ZMesh{}
	self.Mesh_params = params
	self.Mesh_offsets = []float64{0., 0.}
	logger.Debug("bed_mesh: probe/mesh parameters:")
	for key, val := range self.Mesh_params {
		logger.Debugf("%s :  %v", key, val)
	}
	self.Mesh_x_min = params["min_x"].(float64)
	self.Mesh_x_max = params["max_x"].(float64)
	self.Mesh_y_min = params["min_y"].(float64)
	self.Mesh_y_max = params["max_y"].(float64)
	logger.Debugf("bed_mesh: Mesh Min: (%.2f,%.2f) Mesh Max: (%.2f,%.2f)", self.Mesh_x_min, self.Mesh_x_max, self.Mesh_y_min, self.Mesh_y_max)
	interpolationAlgos := map[string]func([][]float64){
		"lagrange": self.Sample_lagrange,
		"bicubic":  self.Sample_bicubic,
		"direct":   self.Sample_direct,
	}
	self.Sample = interpolationAlgos[params["algo"].(string)]
	mesh_x_pps := params["mesh_x_pps"].(int)
	mesh_y_pps := params["mesh_y_pps"].(int)
	px_cnt := params["x_count"].(int)
	py_cnt := params["y_count"].(int)
	self.Mesh_x_count = (px_cnt-1)*mesh_x_pps + px_cnt
	self.Mesh_y_count = (py_cnt-1)*mesh_y_pps + py_cnt
	self.X_mult = mesh_x_pps + 1
	self.Y_mult = mesh_y_pps + 1
	logger.Debugf("bed_mesh: Mesh grid size - X:%d, Y:%d", self.Mesh_x_count, self.Mesh_y_count)
	self.Mesh_x_dist = (self.Mesh_x_max - self.Mesh_x_min) / float64(self.Mesh_x_count-1)
	self.Mesh_y_dist = (self.Mesh_y_max - self.Mesh_y_min) / float64(self.Mesh_y_count-1)
	return self
}

func (self *ZMesh) Get_mesh_matrix() [][]float64 {
	if self.Mesh_matrix != nil {
		var arr [][]float64
		for _, line := range self.Mesh_matrix {
			var round_arr []float64
			for _, z := range line {
				round_arr = append(round_arr, math.Round(z*1000000)/1000000)
			}
			arr = append(arr, round_arr)
		}
		return arr
	}
	return nil
}

func (self *ZMesh) Get_probed_matrix() [][]float64 {
	if self.Probed_matrix != nil {
		var arr [][]float64
		for _, line := range self.Probed_matrix {
			var round_arr []float64
			for _, z := range line {
				round_arr = append(round_arr, math.Round(z*1000000)/1000000)
			}
			arr = append(arr, round_arr)
		}
		return arr
	}
	return nil
}

func (self *ZMesh) Get_mesh_params() map[string]interface{} {
	return self.Mesh_params
}

func (self *ZMesh) Print_probed_matrix(print_func func(string, bool)) {
	if self.Probed_matrix != nil {
		msg := "Mesh Leveling Probed Z positions:\n"
		for _, line := range self.Probed_matrix {
			for _, x := range line {
				msg += fmt.Sprintf(" %f", x)
			}
			msg += "\n"
		}
		print_func(msg, true)
	} else {
		print_func("bed_mesh: bed has not been probed", true)
	}
}

func (self *ZMesh) Print_mesh(print_func func(v ...interface{}), move_z *int) {
	matrix := self.Get_mesh_matrix()
	if matrix != nil {
		var msg strings.Builder
		msg.WriteString(fmt.Sprintf("Mesh X,Y: %d,%d\n", self.Mesh_x_count, self.Mesh_y_count))
		if move_z != nil {
			msg.WriteString(fmt.Sprintf("Search Height: %d\n", *move_z))
		}
		msg.WriteString(fmt.Sprintf("Mesh Offsets: X=%.4f, Y=%.4f\n", self.Mesh_offsets[0], self.Mesh_offsets[1]))
		msg.WriteString(fmt.Sprintf("Mesh Average: %.2f\n", self.Avg_z))
		mesh_min, mesh_max := self.Get_z_range()
		rng := []float64{mesh_min, mesh_max}
		msg.WriteString(fmt.Sprintf("Mesh Range: min=%.4f max=%.4f\n", rng[0], rng[1]))
		msg.WriteString(fmt.Sprintf("Interpolation Algorithm: %s\n", self.Mesh_params["algo"]))
		msg.WriteString("Measured points:\n")
		for y_line := self.Mesh_y_count - 1; y_line >= 0; y_line-- {
			for _, z := range matrix[y_line] {
				msg.WriteString(fmt.Sprintf(" %f", z))
			}
			msg.WriteString("\n")
		}
		print_func(msg.String())
	} else {
		print_func("bed_mesh: Z Mesh not generated")
	}
}

func (self *ZMesh) Build_mesh(z_matrix [][]float64) {
	self.Probed_matrix = z_matrix
	self.Sample(z_matrix)
	sum_val := 0.
	len_val := 0
	for _, x := range self.Mesh_matrix {
		sum := 0.
		for _, item := range x {
			sum += item
		}
		sum_val += sum
		len_val += len(x)
	}
	self.Avg_z = sum_val / float64(len_val)
	self.Avg_z = math.Round(self.Avg_z*1000) / 1000
}

func (self *ZMesh) Set_mesh_offsets(offsets []float64) {
	for i, o := range offsets {
		if o != 0.0 {
			self.Mesh_offsets[i] = o
		}
	}
}

func (self *ZMesh) Get_x_coordinate(index int) float64 {
	return self.Mesh_x_min + self.Mesh_x_dist*float64(index)
}

func (self *ZMesh) Get_y_coordinate(index int) float64 {
	return self.Mesh_y_min + self.Mesh_y_dist*float64(index)
}

func (self *ZMesh) Calc_z(x, y float64) float64 {
	if self.Mesh_matrix != nil {
		tbl := self.Mesh_matrix
		tx, xidx := self.Get_linear_index(x+self.Mesh_offsets[0], 0)
		ty, yidx := self.Get_linear_index(y+self.Mesh_offsets[1], 1)
		z0 := Lerp(tx, tbl[yidx][xidx], tbl[yidx][xidx+1])
		z1 := Lerp(tx, tbl[yidx+1][xidx], tbl[yidx+1][xidx+1])
		return Lerp(ty, z0, z1)
	}
	return 0.
}

func (self *ZMesh) Get_z_range() (float64, float64) {
	if self.Mesh_matrix != nil {
		mesh_min := 0.
		mesh_max := 0.
		for _, x := range self.Mesh_matrix {
			min := 0.
			max := 0.
			for _, item := range x {
				if item < min {
					min = item
				}
				if item > max {
					max = item
				}
			}
			if min < mesh_min {
				mesh_min = min
			}
			if max > mesh_max {
				mesh_max = max
			}
		}
		return mesh_min, mesh_max
	}
	return 0., 0.
}

func (self *ZMesh) Get_linear_index(coord float64, axis int) (float64, int) {
	var meshMin float64
	var meshCnt int
	var meshDist float64
	var cfunc func(int) float64
	if axis == 0 {
		meshMin = self.Mesh_x_min
		meshCnt = self.Mesh_x_count
		meshDist = self.Mesh_x_dist
		cfunc = self.Get_x_coordinate
	} else {
		meshMin = self.Mesh_y_min
		meshCnt = self.Mesh_y_count
		meshDist = self.Mesh_y_dist
		cfunc = self.Get_y_coordinate
	}
	idx := int(math.Floor((coord - meshMin) / meshDist))
	idx = int(Constrain(float64(idx), 0., float64(meshCnt-2)))
	t := (coord - cfunc(idx)) / meshDist
	return Constrain(t, 0., 1.), idx
}

func (self *ZMesh) Sample_direct(zMatrix [][]float64) {
	self.Mesh_matrix = zMatrix
}

func (self *ZMesh) Sample_lagrange(z_matrix [][]float64) {
	x_mult := self.X_mult
	y_mult := self.Y_mult
	for j := 0; j < self.Mesh_y_count; j++ {
		var arr []float64
		for i := 0; i < self.Mesh_x_count; i++ {
			val := 0.
			if (i%x_mult) != 0 || j%y_mult != 0 {
				val = 0
			} else {
				val = z_matrix[j/y_mult][i/x_mult]
			}
			arr = append(arr, val)
		}
		self.Mesh_matrix = append(self.Mesh_matrix, arr)
	}
	xpts, ypts := self.Get_lagrange_coords()
	for i := 0; i < self.Mesh_y_count; i++ {
		if i%y_mult != 0 {
			continue
		}
		for j := 0; j < self.Mesh_x_count; j++ {
			if j%x_mult == 0 {
				continue
			}
			x := self.Get_x_coordinate(j)
			self.Mesh_matrix[i][j] = self.Calc_lagrange(xpts, x, i, 0)
		}
	}
	for i := 0; i < self.Mesh_x_count; i++ {
		for j := 0; j < self.Mesh_y_count; j++ {
			if j%y_mult == 0 {
				continue
			}
			y := self.Get_y_coordinate(j)
			self.Mesh_matrix[j][i] = self.Calc_lagrange(ypts, y, i, 1)
		}
	}
}

func (self *ZMesh) Get_lagrange_coords() ([]float64, []float64) {
	var xpts, ypts []float64
	for i := 0; i < self.Mesh_params["x_count"].(int); i++ {
		xpts = append(xpts, self.Get_x_coordinate(i*self.X_mult))
	}
	for j := 0; j < self.Mesh_params["y_count"].(int); j++ {
		ypts = append(ypts, self.Get_y_coordinate(j*self.Y_mult))
	}
	return xpts, ypts
}

func (self *ZMesh) Calc_lagrange(lpts []float64, c float64, vec int, axis int) float64 {
	pt_cnt := len(lpts)
	total := 0.
	for i := 0; i < pt_cnt; i++ {
		n := 1.
		d := 1.
		for j := 0; j < pt_cnt; j++ {
			if j == i {
				continue
			}
			n *= c - lpts[j]
			d *= lpts[i] - lpts[j]
		}
		var z float64
		if axis == 0 {
			z = self.Mesh_matrix[vec][i*self.X_mult]
		} else {
			z = self.Mesh_matrix[i*self.Y_mult][vec]
		}
		total += z * n / d
	}
	return total
}

func (self *ZMesh) Sample_bicubic(z_matrix [][]float64) {
	x_mult := self.X_mult
	y_mult := self.Y_mult
	c := self.Mesh_params["tension"]
	for j := 0; j < self.Mesh_y_count; j++ {
		var arr []float64
		for i := 0; i < self.Mesh_x_count; i++ {
			val := 0.
			if i%x_mult != 0 || j%y_mult != 0 {
				val = 0.
			} else {
				val = z_matrix[j/y_mult][i/x_mult]
			}
			arr = append(arr, val)
		}
		self.Mesh_matrix = append(self.Mesh_matrix, arr)
	}
	for y := 0; y < self.Mesh_y_count; y++ {
		if y%y_mult != 0 {
			continue
		}
		for x := 0; x < self.Mesh_x_count; x++ {
			if x%x_mult == 0 {
				continue
			}
			pts := self.Get_x_ctl_pts(x, y)
			self.Mesh_matrix[y][x] = self.Cardinal_spline(pts, c.(float64))
		}
	}
	for x := 0; x < self.Mesh_x_count; x++ {
		for y := 0; y < self.Mesh_y_count; y++ {
			if y%y_mult == 0 {
				continue
			}
			pts := self.Get_y_ctl_pts(x, y)
			self.Mesh_matrix[y][x] = self.Cardinal_spline(pts, c.(float64))
		}
	}
}

func (self *ZMesh) Get_x_ctl_pts(x int, y int) []float64 {
	x_mult := self.X_mult
	x_row := self.Mesh_matrix[y]
	last_pt := self.Mesh_x_count - 1 - x_mult
	var p0, p1, p2, p3, t float64
	if x < x_mult {
		p0 = x_row[0]
		p1 = x_row[0]
		p2 = x_row[x_mult]
		p3 = x_row[2*x_mult]
		t = float64(x) / float64(x_mult)
	} else if x > last_pt {
		p0 = x_row[last_pt-x_mult]
		p1 = x_row[last_pt]
		p2 = x_row[last_pt+x_mult]
		p3 = x_row[last_pt+x_mult]
		t = float64(x-last_pt) / float64(x_mult)
	} else {
		found := false
		for i := x_mult; i < last_pt; i += x_mult {
			if x > i && x < i+x_mult {
				p0 = x_row[i-x_mult]
				p1 = x_row[i]
				p2 = x_row[i+x_mult]
				p3 = x_row[i+2*x_mult]
				t = float64(x-i) / float64(x_mult)
				found = true
				break
			}
		}
		if !found {
			panic(&BedMeshError{"bed_mesh: Error finding x control points"})
		}
	}
	return []float64{p0, p1, p2, p3, t}
}

func (self *ZMesh) Get_y_ctl_pts(x int, y int) []float64 {
	y_mult := self.Y_mult
	last_pt := self.Mesh_y_count - 1 - y_mult
	y_col := self.Mesh_matrix
	var p0, p1, p2, p3, t float64
	if y < y_mult {
		p0 = y_col[0][x]
		p1 = y_col[0][x]
		p2 = y_col[y_mult][x]
		p3 = y_col[2*y_mult][x]
		t = float64(y) / float64(y_mult)
	} else if y > last_pt {
		p0 = y_col[last_pt-y_mult][x]
		p1 = y_col[last_pt][x]
		p2 = y_col[last_pt+y_mult][x]
		p3 = y_col[last_pt+y_mult][x]
		t = float64(y-last_pt) / float64(y_mult)
	} else {
		found := false
		for i := y_mult; i < last_pt; i += y_mult {
			if y > 1 && y < i+y_mult {
				p0 = y_col[i-y_mult][x]
				p1 = y_col[i][x]
				p2 = y_col[i+y_mult][x]
				p3 = y_col[i+2*y_mult][x]
				t = float64(y-i) / float64(y_mult)
				found = true
				break
			}
		}
		if !found {
			panic(&BedMeshError{"bed_mesh: Error finding y control points"})
		}
	}
	return []float64{p0, p1, p2, p3, t}
}

func (self *ZMesh) Cardinal_spline(p []float64, tension float64) float64 {
	t := p[4]
	t2 := t * t
	t3 := t2 * t
	m1 := tension * (p[2] - p[0])
	m2 := tension * (p[3] - p[1])
	a := p[1] * (2*t3 - 3*t2 + 1)
	b := p[2] * (-2*t3 + 3*t2)
	c := m1 * (t3 - 2*t2 + t)
	d := m2 * (t3 - t2)
	return a + b + c + d
}
