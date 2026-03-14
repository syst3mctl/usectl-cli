package api

// BillingStatus represents the user's billing information.
type BillingStatus struct {
	Plan               string  `json:"plan"`
	SubscriptionStatus string  `json:"subscription_status"`
	TrialEndsAt        *string `json:"trial_ends_at,omitempty"`
	IsFoundingMember   bool    `json:"is_founding_member"`
	StripeCustomerID   *string `json:"stripe_customer_id,omitempty"`
}

// GetBillingStatus returns the authenticated user's billing status.
func (c *Client) GetBillingStatus() (*BillingStatus, error) {
	var status BillingStatus
	if err := c.Get("/api/billing/status", &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// CreateCheckoutSession creates a Stripe checkout session.
func (c *Client) CreateCheckoutSession(plan, successURL, cancelURL string) (string, error) {
	var result struct {
		URL string `json:"url"`
	}
	body := map[string]string{
		"plan":        plan,
		"success_url": successURL,
		"cancel_url":  cancelURL,
	}
	if err := c.Post("/api/billing/checkout", body, &result); err != nil {
		return "", err
	}
	return result.URL, nil
}

// CreateBillingPortal creates a Stripe billing portal session.
func (c *Client) CreateBillingPortal(returnURL string) (string, error) {
	var result struct {
		URL string `json:"url"`
	}
	body := map[string]string{"return_url": returnURL}
	if err := c.Post("/api/billing/portal", body, &result); err != nil {
		return "", err
	}
	return result.URL, nil
}
