// Package middleware provides shared Gin middleware for marketplace-api.
package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders adds standard security response headers to every response.
// Covers OWASP recommended headers: HSTS, CSP, X-Content-Type-Options,
// X-Frame-Options, Referrer-Policy.
func SecurityHeaders() gin.HandlerFunc {
	csp := "default-src 'self'; " +
		"script-src 'self' https://js.stripe.com https://checkout.razorpay.com; " +
		"frame-src https://js.stripe.com https://api.razorpay.com; " +
		"img-src 'self' https://storage.googleapis.com data:; " +
		"style-src 'self' 'unsafe-inline'; " +
		"connect-src 'self' https://api.stripe.com"

	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "0")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Header("Content-Security-Policy", csp)
		c.Next()
	}
}
