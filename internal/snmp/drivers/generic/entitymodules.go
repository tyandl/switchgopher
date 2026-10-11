package generic

import (
	"github.com/gosnmp/gosnmp"

	"github.com/tyandl/switchgopher/internal/model"
	"github.com/tyandl/switchgopher/internal/snmp"
)

func init() {
	snmp.RegisterEntityModules(snmp.VendorGeneric, entityModulesFetcher{})
}

type entityModulesFetcher struct{}

// FetchEntityModules returns one Module per line-card slot in a modular
// chassis - the same entPhysicalTable walk used to identify the chassis
// itself (see chassisRow), filtered to entPhysicalClass == module(9)
// instead of chassis(3). Device is left blank here, filled in by the
// caller like Port/Node.
func (entityModulesFetcher) FetchEntityModules(s *gosnmp.GoSNMP) ([]model.Module, error) {
	var modules []model.Module
	for idx, row := range walkEntPhysicalTable(s) {
		if row.Class != entPhysicalClassModule {
			continue
		}
		modules = append(modules, model.Module{
			Module:      idx,
			Description: row.Descr,
			Model:       row.ModelName,
			FwVersion:   row.FirmwareRev,
			HwVersion:   row.HardwareRev,
			SwVersion:   row.SoftwareRev,
			SerialNum:   row.SerialNum,
		})
	}
	return modules, nil
}
