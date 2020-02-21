// Package graphql-go-tools is library to create GraphQL services using the go programming language.
//
// About GraphQL
//
// GraphQL is a query language for APIs and a runtime for fulfilling those queries with your existing data. GraphQL provides a complete and understandable description of the data in your API, gives clients the power to ask for exactly what they need and nothing more, makes it easier to evolve APIs over time, and enables powerful developer tools.
//
// Source: https://graphql.org
//
// About this library
//
// This library is intended to be a set of low level building blocks to write high performance and secure GraphQL applications.
// Use cases could range from writing layer seven GraphQL proxies, firewalls, caches etc..
// You would usually not use this library to write a GraphQL server yourself but to build tools for the GraphQL ecosystem.
//
// To achieve this goal the library has zero dependencies at its core functionality.
// It has a full implementation of the GraphQL AST and supports lexing, parsing, validation, normalization, introspection, query planning as well as query execution etc.
//
// With the execution package it's possible to write a fully functional GraphQL server that is capable to mediate between various protocols and formats.
// In it's current state you can use the following DataSources to resolve fields:
// - Static data (embed static data into a schema to extend a field in a simple way)
// - HTTP JSON APIs (combine multiple Restful APIs into one single GraphQL Endpoint, nesting is possible)
// - GraphQL APIs (you can combine multiple GraphQL APIs into one single GraphQL Endpoint, nesting is possible)
// - Webassembly/WASM Lambdas (e.g. resolve a field using a Rust lambda)
//
// If you're looking for a ready to use solution that has all this functionality packaged as a Gateway have a look at: https://github.com/jensneuse/graphql-gateway
//
// Created by Jens Neuse
package main
