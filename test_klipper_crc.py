import tmc_uart

class MockTMC:
    def _calc_crc8(self, data):
        return tmc_uart.MCU_TMC_uart_bitbang._calc_crc8(self, data)
    def _add_serial_bits(self, data):
        return tmc_uart.MCU_TMC_uart_bitbang._add_serial_bits(self, data)

tmc = MockTMC()
out = tmc_uart.MCU_TMC_uart_bitbang._encode_read(tmc, 0xf5, 0xff, 0x02)
print("Python encode_read(0xf5, 0xff, 0x02):", list(out))

out2 = tmc_uart.MCU_TMC_uart_bitbang._encode_read(tmc, 0x05, 0xff, 0x02)
print("Python encode_read(0x05, 0xff, 0x02):", list(out2))
