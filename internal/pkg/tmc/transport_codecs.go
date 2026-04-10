package tmc

import (
	"goklipper/common/utils/maths"
	"math/big"
	"reflect"
)

func CalcCRC8ATM(data []int64) int64 {
	var crc int64 = 0
	for _, b := range data {
		for i := 0; i < 8; i++ {
			if (crc>>7)^(b&0x01) != 0 {
				crc = (crc << 1) ^ 0x07
			} else {
				crc = crc << 1
			}
			crc &= 0xff
			b >>= 1
		}
	}
	return crc
}

func AddSerialBits(data []int64) []int64 {
	out := big.NewInt(0)
	var pos uint = 0
	for _, d := range data {
		b := big.NewInt(d)
		b.Lsh(b, 1).Or(b, big.NewInt(int64(0x200)))
		out.Or(out, b.Lsh(b, pos))
		pos += 10
	}

	res := make([]int64, 0)
	end := maths.FloorDiv(int(pos+7), 8)
	for i := 0; i < end; i++ {
		t := big.NewInt(0)
		t.Set(out)
		t = t.Rsh(t, uint(i*8)).And(t, big.NewInt(0xff))
		res = append(res, t.Int64())
	}
	return res
}

func EncodeUARTRead(sync, addr, reg int64) []int64 {
	msg := []int64{sync, addr, reg}
	msg = append(msg, CalcCRC8ATM(msg))
	return AddSerialBits(msg)
}

func EncodeUARTWrite(sync, addr, reg, val int64) []int64 {
	msg := []int64{sync, addr, reg, (val >> 24) & 0xff, (val >> 16) & 0xff, (val >> 8) & 0xff, val & 0xff}
	msg = append(msg, CalcCRC8ATM(msg))
	return AddSerialBits(msg)
}

func DecodeUARTRead(reg int64, data []int64) (int64, bool) {
	if len(data) != 10 {
		return 0, false
	}
	mval := big.NewInt(0)
	de := big.NewInt(0)
	var pos uint = 0
	for _, d := range data {
		de.SetUint64(uint64(d))
		de.Lsh(de, pos)
		mval.Or(mval, de)
		pos += 8
	}

	ff := big.NewInt(0)
	ff.SetUint64(uint64(0xff))
	t31 := big.NewInt(0)
	t31.Set(mval)
	t31.Rsh(t31, 31)
	t31.And(t31, ff)
	t31.Lsh(t31, 24)

	t41 := big.NewInt(0)
	t41.Set(mval)
	t41.Rsh(t41, 41)
	t41.And(t41, ff)
	t41.Lsh(t41, 16)

	t51 := big.NewInt(0)
	t51.Set(mval)
	t51.Rsh(t51, 51)
	t51.And(t51, ff)
	t51.Lsh(t51, 8)

	t61 := big.NewInt(0)
	t61.Set(mval)
	t61.Rsh(t61, 61)
	t61.And(t61, ff)

	val := int64(t31.Uint64() | t41.Uint64() | t51.Uint64() | t61.Uint64())
	if !reflect.DeepEqual(data, EncodeUARTWrite(0x05, 0xff, reg, val)) {
		return 0, false
	}
	return val, true
}

func BuildSPIChainCommand(data []int64, chainLen, chainPos int64) []int {
	cmd := make([]int, 0)
	for i := 0; i < int(chainLen-chainPos)*5; i++ {
		cmd = append(cmd, 0x00)
	}
	for _, d := range data {
		cmd = append(cmd, int(d))
	}
	for i := 0; i < int(chainPos-1)*5; i++ {
		cmd = append(cmd, 0x00)
	}
	return cmd
}

func DecodeSPIChainResponse(response string, chainLen, chainPos int64) int64 {
	pr := []rune(response)
	pr = pr[(chainLen-chainPos)*5 : (chainLen-chainPos+1)*5]
	return int64((pr[1] << 24) | (pr[2] << 16) | (pr[3] << 8) | pr[4])
}