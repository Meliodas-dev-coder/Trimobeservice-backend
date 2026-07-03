package orders

import "strings"

func validateAddCartItem(req AddCartItemRequest) map[string]string {
	p := map[string]string{}
	if req.ProductVariantID <= 0 {
		p["product_variant_id"] = "is required"
	}
	if req.Quantity <= 0 {
		p["quantity"] = "must be at least 1"
	}
	return p
}

func validateUpdateCartItem(req UpdateCartItemRequest) map[string]string {
	p := map[string]string{}
	if req.Quantity <= 0 {
		p["quantity"] = "must be at least 1"
	}
	return p
}

func validateCheckout(req CheckoutRequest) map[string]string {
	p := map[string]string{}
	switch req.FulfillmentType {
	case FulfillmentDelivery:
		a := req.ShippingAddress
		if a == nil {
			p["shipping_address"] = "is required for delivery"
			break
		}
		if strings.TrimSpace(a.RecipientName) == "" {
			p["shipping_address.recipient_name"] = "is required"
		}
		if strings.TrimSpace(a.Phone) == "" {
			p["shipping_address.phone"] = "is required"
		}
		if strings.TrimSpace(a.Line1) == "" {
			p["shipping_address.line1"] = "is required"
		}
		if strings.TrimSpace(a.City) == "" {
			p["shipping_address.city"] = "is required"
		}
		if strings.TrimSpace(a.Country) == "" {
			p["shipping_address.country"] = "is required"
		}
	case FulfillmentPickup:
		// no address needed
	default:
		p["fulfillment_type"] = "must be 'delivery' or 'pickup'"
	}
	return p
}
