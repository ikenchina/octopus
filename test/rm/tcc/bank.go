package tcc

import "gorm.io/gorm"

func StartRm(listen string, db *gorm.DB, protocol string) error {
	if protocol == "http" {
		return NewTccRmBankService(listen, db).Start()
	}
	return NewTccRmBankGrpcService(listen, db).Start()
}
