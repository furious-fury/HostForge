// Package caddy reconciles domains onto Caddy's running configuration,
// eventually rather than synchronously (ADR-0002 §6).
//
// Before this package existed, the deploy path called caddy.Sync inline: a
// reload failure rolled the service's active deployment back to the
// previous one, marked the just-finished deploy failed, and tore down a
// container that was running and passing health checks -- all because of
// an edge-routing hiccup unrelated to whether the deploy itself succeeded.
// A healthy container with no published route is unpublished, not failed.
package caddy
