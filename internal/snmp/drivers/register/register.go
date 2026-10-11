// Package register is the one place that knows every vendor fetch package
// exists. Its blank imports trigger each package's init(), which registers
// its implementations into internal/snmp's registry - the same
// self-registering pattern database/sql drivers use. Core internal/snmp
// never imports vendor packages directly (that would risk a cycle, since
// vendor packages import inward on internal/snmp for the interfaces to
// implement); cmd/switchgopher blank-imports this package once at startup
// instead.
package register

import (
	_ "github.com/tyandl/switchgopher/internal/snmp/drivers/cisco"
	_ "github.com/tyandl/switchgopher/internal/snmp/drivers/generic"
)
