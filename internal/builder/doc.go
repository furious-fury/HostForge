// Package builder defines the builder-neutral deployment contract.
//
// It deliberately contains no active builder implementation. The deployment
// service continues to use Nixpacks until a Railpack/BuildKit adapter satisfies
// this contract and is wired in through a later migration change.
package builder
