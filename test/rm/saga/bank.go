package saga

import "gorm.io/gorm"

func StartRm(listen string, db *gorm.DB, protocol string) error {
	if protocol == "http" {
		return NewSagaRmBankService(listen, db).Start()
	}
	return NewSagaRmBankGrpcService(listen, db).Start()
}
