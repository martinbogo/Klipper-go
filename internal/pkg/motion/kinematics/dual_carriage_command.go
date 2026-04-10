package kinematics

// SetDualCarriageHelp is the help string for the SET_DUAL_CARRIAGE G-code command.
const SetDualCarriageHelp = "Set which carriage is active"

// DualCarriageCommand is the interface for the SET_DUAL_CARRIAGE G-code command parameter object.
type DualCarriageCommand interface {
	Get_int(name string, _default interface{}, minval *int, maxval *int) int
}

// HandleSetDualCarriageCommand reads the CARRIAGE parameter and activates the requested
// carriage on the given CartesianKinematics. It returns nil on success.
func HandleSetDualCarriageCommand(kin *CartesianKinematics, cmd DualCarriageCommand) error {
	minval := 0
	maxval := 1
	carriage := cmd.Get_int("CARRIAGE", nil, &minval, &maxval)
	kin.ActivateCarriage(carriage)
	return nil
}
