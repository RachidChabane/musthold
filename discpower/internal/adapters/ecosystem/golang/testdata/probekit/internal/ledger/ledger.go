package ledger

import "errors"

var ErrUnbalanced = errors.New("unbalanced")

func Net(gross, tax int) (int, error) {
	if tax > gross {
		return 0, ErrUnbalanced
	}
	return gross - tax, nil
}
