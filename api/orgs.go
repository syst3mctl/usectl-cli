package api

import "fmt"

// ========== Organizations ==========

// Organization represents an organization.
type Organization struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	AvatarURL   string `json:"avatar_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// OrganizationMember represents a member of an organization.
type OrganizationMember struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
	Role      string `json:"role"`
	JoinedAt  string `json:"joined_at"`
}

// OrganizationInvitation represents a pending invitation.
type OrganizationInvitation struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	Email          string  `json:"email"`
	Role           string  `json:"role"`
	Token          string  `json:"token"`
	ExpiresAt      string  `json:"expires_at"`
	InvitedBy      *string `json:"invited_by,omitempty"`
	CreatedAt      string  `json:"created_at"`
}

// InvitationInfo is the public info returned for a pending invitation token.
type InvitationInfo struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	OrgName        string `json:"org_name"`
	Email          string `json:"email"`
	Role           string `json:"role"`
	ExpiresAt      string `json:"expires_at"`
}

// AcceptInvitationResponse is the response after accepting an invitation.
type AcceptInvitationResponse struct {
	Message string `json:"message"`
	OrgName string `json:"org_name"`
}

// CreateOrganizationRequest is the input for creating an organization.
type CreateOrganizationRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description,omitempty"`
}

// UpdateOrganizationRequest is the input for updating an organization.
type UpdateOrganizationRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// CreateInvitationRequest is the input for creating an invitation.
type CreateInvitationRequest struct {
	Email string `json:"email"`
	Role  string `json:"role,omitempty"`
}

// UpdateMemberRoleRequest is the input for changing a member's role.
type UpdateMemberRoleRequest struct {
	Role string `json:"role"`
}

// ---------- API methods ----------

// ListOrganizations returns all organizations the current user belongs to.
func (c *Client) ListOrganizations() ([]Organization, error) {
	var orgs []Organization
	err := c.Get("/api/organizations", &orgs)
	return orgs, err
}

// GetOrganization returns a single organization by ID.
func (c *Client) GetOrganization(id string) (*Organization, error) {
	var org Organization
	err := c.Get(fmt.Sprintf("/api/organizations/%s", id), &org)
	return &org, err
}

// CreateOrganization creates a new organization.
func (c *Client) CreateOrganization(req CreateOrganizationRequest) (*Organization, error) {
	var org Organization
	err := c.Post("/api/organizations", req, &org)
	return &org, err
}

// UpdateOrganization updates an existing organization.
func (c *Client) UpdateOrganization(id string, req UpdateOrganizationRequest) (*Organization, error) {
	var org Organization
	err := c.Put(fmt.Sprintf("/api/organizations/%s", id), req, &org)
	return &org, err
}

// DeleteOrganization deletes an organization.
func (c *Client) DeleteOrganization(id string) error {
	return c.Delete(fmt.Sprintf("/api/organizations/%s", id), nil)
}

// ListOrgMembers returns members of an organization.
func (c *Client) ListOrgMembers(orgID string) ([]OrganizationMember, error) {
	var members []OrganizationMember
	err := c.Get(fmt.Sprintf("/api/organizations/%s/members", orgID), &members)
	return members, err
}

// UpdateMemberRole changes a member's role in an organization.
func (c *Client) UpdateMemberRole(orgID, userID string, req UpdateMemberRoleRequest) error {
	return c.Put(fmt.Sprintf("/api/organizations/%s/members/%s/role", orgID, userID), req, nil)
}

// RemoveMember removes a member from an organization.
func (c *Client) RemoveMember(orgID, userID string) error {
	return c.Delete(fmt.Sprintf("/api/organizations/%s/members/%s", orgID, userID), nil)
}

// ListInvitations returns pending invitations for an organization.
func (c *Client) ListInvitations(orgID string) ([]OrganizationInvitation, error) {
	var invs []OrganizationInvitation
	err := c.Get(fmt.Sprintf("/api/organizations/%s/invitations", orgID), &invs)
	return invs, err
}

// CreateInvitation creates a new invitation for an organization.
func (c *Client) CreateInvitation(orgID string, req CreateInvitationRequest) (*OrganizationInvitation, error) {
	var inv OrganizationInvitation
	err := c.Post(fmt.Sprintf("/api/organizations/%s/invitations", orgID), req, &inv)
	return &inv, err
}

// RevokeInvitation revokes a pending invitation.
func (c *Client) RevokeInvitation(orgID, invID string) error {
	return c.Delete(fmt.Sprintf("/api/organizations/%s/invitations/%s", orgID, invID), nil)
}

// ListOrgProjects returns projects belonging to an organization.
func (c *Client) ListOrgProjects(orgID string) ([]Project, error) {
	var projects []Project
	err := c.Get(fmt.Sprintf("/api/organizations/%s/projects", orgID), &projects)
	return projects, err
}

// GetInvitationInfo returns public details of a pending invitation by token.
func (c *Client) GetInvitationInfo(token string) (*InvitationInfo, error) {
	var info InvitationInfo
	err := c.Get(fmt.Sprintf("/api/invitations/%s", token), &info)
	return &info, err
}

// AcceptInvitation accepts a pending invitation by token.
func (c *Client) AcceptInvitation(token string) (*AcceptInvitationResponse, error) {
	var resp AcceptInvitationResponse
	err := c.Post(fmt.Sprintf("/api/invitations/%s/accept", token), nil, &resp)
	return &resp, err
}
