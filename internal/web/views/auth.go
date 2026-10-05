package web_views

// AuthFormData contains only values that may be returned to the browser.
// Passwords must never be included here.
type AuthFormData struct {
	Username  string
	FirstName string
	Message   string
	Errors    map[string]string
}
