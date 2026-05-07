package api

import "fmt"

// ========== Notifications ==========

type Notification struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Read      bool   `json:"read"`
	CreatedAt string `json:"created_at"`
}

func (c *Client) ListNotifications() ([]Notification, error) {
	var notifs []Notification
	err := c.Get("/api/notifications", &notifs)
	return notifs, err
}

func (c *Client) MarkNotificationRead(id string) error {
	return c.Put(fmt.Sprintf("/api/notifications/%s/read", id), nil, nil)
}

func (c *Client) MarkAllNotificationsRead() error {
	return c.Post("/api/notifications/read-all", nil, nil)
}

func (c *Client) UnreadNotificationCount() (int, error) {
	var resp struct {
		Count int `json:"count"`
	}
	err := c.Get("/api/notifications/unread-count", &resp)
	return resp.Count, err
}
