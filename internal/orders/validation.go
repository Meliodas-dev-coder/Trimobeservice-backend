package orders

import (
	"fmt"
	"strings"
)

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
	validateFulfillment(p, req.FulfillmentType, req.ShippingAddress)
	return p
}

func validateAdminCreateOrder(req AdminCreateOrderRequest) map[string]string {
	p := map[string]string{}
	if strings.TrimSpace(req.CustomerName) == "" {
		p["customer_name"] = "is required"
	}
	if len(req.Items) == 0 {
		p["items"] = "add at least one item"
	}
	for i, li := range req.Items {
		if li.ProductVariantID <= 0 {
			p[fmt.Sprintf("items.%d.product_variant_id", i)] = "is required"
		}
		if li.Quantity <= 0 {
			p[fmt.Sprintf("items.%d.quantity", i)] = "must be at least 1"
		}
	}
	validateFulfillment(p, req.FulfillmentType, req.ShippingAddress)
	return p
}

// validateFulfillment applies the shared delivery/pickup + address rules.
func validateFulfillment(p map[string]string, fulfillment string, a *ShippingAddress) {
	switch fulfillment {
	case FulfillmentDelivery:
		if a == nil {
			p["shipping_address"] = "is required for delivery"
			return
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
		validateCoordinates(p, "shipping_address", a.Latitude, a.Longitude)
	case FulfillmentPickup:
		// no address needed
	default:
		p["fulfillment_type"] = "must be 'delivery' or 'pickup'"
	}
}

func validateCoordinates(p map[string]string, prefix string, latitude, longitude *float64) {
	if (latitude == nil) != (longitude == nil) {
		p[prefix+".coordinates"] = "latitude and longitude must be provided together"
		return
	}
	if latitude == nil {
		return
	}
	if *latitude < -90 || *latitude > 90 {
		p[prefix+".latitude"] = "must be between -90 and 90"
	}
	if *longitude < -180 || *longitude > 180 {
		p[prefix+".longitude"] = "must be between -180 and 180"
	}
}
