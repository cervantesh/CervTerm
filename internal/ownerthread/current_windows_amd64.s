//go:build windows && amd64

#include "textflag.h"

TEXT ·current(SB), NOSPLIT, $0-8
	MOVQ 0x48(GS), AX
	MOVQ AX, ret+0(FP)
	RET
