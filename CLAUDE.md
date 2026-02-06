# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Entitlements-as-a-Service is a telecommunications entitlement server that authenticates mobile devices using SIM card hardware tokens via the EAP-AKA (Extensible Authentication Protocol - Authentication and Key Agreement) protocol over HTTP/HTTPS. The system follows the GSMA TS.43 standard.

## Requirements

Protocol Compliance (GSMA TS.43 / EAP-AKA protocol as specified in RFC 4187). All the implementation in this project shall be in compliance with the specification. Do not re-invent any solution if it already is standardized in the spec.

## Project Knowledge

See [JOURNAL.md](JOURNAL.md) for accumulated architectural decisions, gotchas, and lessons learned. Consult it before making changes to avoid repeating past mistakes.

## Coding Best Practices
Do not keep any dead code around. If you find dead code from previous iterations that is not exercised any more, refactor and clean them up. Always run all unit and integration tests after removing dead code.