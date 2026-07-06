package payments

import (
	"regexp"
	"strings"
)

var amountRe = regexp.MustCompile(`^\d{1,10}(\.\d{1,2})?$`)

var validMethods = map[string]bool{
	MethodCash:         true,
	MethodBankTransfer: true,
	MethodMobileMoney:  true,
	MethodOther:        true,
}

func validateRecord(req RecordPaymentRequest) map[string]string {
	p := map[string]string{}
	if req.PayableType != PayableOrder && req.PayableType != PayableBooking && req.PayableType != PayableEvent {
		p["payable_type"] = "must be 'order', 'booking', or 'event'"
	}
	if req.PayableID <= 0 {
		p["payable_id"] = "is required"
	}
	if !validMethods[req.Method] {
		p["method"] = "must be one of cash, bank_transfer, mobile_money, other"
	}
	if req.Amount != nil {
		if a := strings.TrimSpace(*req.Amount); a != "" && !amountRe.MatchString(a) {
			p["amount"] = "must be a decimal amount, e.g. 120.00"
		}
	}
	return p
}
