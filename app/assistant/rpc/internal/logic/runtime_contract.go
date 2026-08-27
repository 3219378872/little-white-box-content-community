package logic

import "esx/pkg/errx"

func requireAgentUser(userID int64) error {
	if userID <= 0 {
		return errx.NewWithCode(errx.LoginRequired)
	}
	return nil
}

func unavailableUntilStore() error {
	return errx.NewWithCode(errx.ServiceUnavailable)
}
