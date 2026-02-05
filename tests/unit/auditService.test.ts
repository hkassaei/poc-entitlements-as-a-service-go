import { describe, it, expect } from 'vitest';
import { extractAuditableRequest } from '../../src/db/auditService.js';

describe('Audit Service', () => {
  describe('extractAuditableRequest', () => {
    describe('allowed fields', () => {
      it('passes through app field unchanged', () => {
        const result = extractAuditableRequest({ app: 'ap2004' }, undefined);
        expect(result.app).toBe('ap2004');
      });

      it('passes through terminal_id unchanged', () => {
        const result = extractAuditableRequest({ terminal_id: '12345678901234' }, undefined);
        expect(result.terminal_id).toBe('12345678901234');
      });

      it('passes through entitlement_version unchanged', () => {
        const result = extractAuditableRequest({ entitlement_version: '2' }, undefined);
        expect(result.entitlement_version).toBe('2');
      });

      it('passes through operation unchanged', () => {
        const result = extractAuditableRequest({ operation: 'ManageSubscription' }, undefined);
        expect(result.operation).toBe('ManageSubscription');
      });

      it('passes through operation_type unchanged', () => {
        const result = extractAuditableRequest({ operation_type: 1 }, undefined);
        expect(result.operation_type).toBe(1);
      });

      it('passes through accept_content_type unchanged', () => {
        const result = extractAuditableRequest({ accept_content_type: 'application/json' }, undefined);
        expect(result.accept_content_type).toBe('application/json');
      });
    });

    describe('redacted fields', () => {
      it('redacts imsi showing character count', () => {
        const result = extractAuditableRequest({ imsi: '001010000000001' }, undefined);
        expect(result.imsi).toBe('[REDACTED:15chars]');
      });

      it('redacts token showing character count', () => {
        const token = 'a'.repeat(64);
        const result = extractAuditableRequest({ token }, undefined);
        expect(result.token).toBe('[REDACTED:64chars]');
      });

      it('redacts eap_relay showing character count', () => {
        // Simulate a base64 EAP packet (~100+ chars)
        const eapRelay = 'A'.repeat(200);
        const result = extractAuditableRequest({ eap_relay: eapRelay }, undefined);
        expect(result.eap_relay).toBe('[REDACTED:200chars]');
      });

      it('redacts null values appropriately', () => {
        const result = extractAuditableRequest({ token: null }, undefined);
        expect(result.token).toBe('[REDACTED:null]');
      });

      it('redacts undefined values appropriately', () => {
        const result = extractAuditableRequest({ token: undefined }, undefined);
        expect(result.token).toBe('[REDACTED:null]');
      });
    });

    describe('omitted fields', () => {
      it('completely omits ki field', () => {
        const result = extractAuditableRequest({ ki: 'secret-key-value' }, undefined);
        expect(result.ki).toBeUndefined();
        expect('ki' in result).toBe(false);
      });

      it('completely omits op field', () => {
        const result = extractAuditableRequest({ op: 'operator-variant' }, undefined);
        expect(result.op).toBeUndefined();
      });

      it('completely omits password field', () => {
        const result = extractAuditableRequest({ password: 'secret123' }, undefined);
        expect(result.password).toBeUndefined();
      });

      it('completely omits secret field', () => {
        const result = extractAuditableRequest({ secret: 'my-secret' }, undefined);
        expect(result.secret).toBeUndefined();
      });
    });

    describe('unknown fields', () => {
      it('omits unknown fields for safety (fail-safe)', () => {
        const result = extractAuditableRequest({
          app: 'ap2004',
          some_new_field: 'potentially-sensitive',
          another_unknown: 12345,
        }, undefined);

        expect(result.app).toBe('ap2004');
        expect(result.some_new_field).toBeUndefined();
        expect(result.another_unknown).toBeUndefined();
      });
    });

    describe('query parameters', () => {
      it('prefixes query parameters with query_', () => {
        const result = extractAuditableRequest(undefined, { app: 'ap2004' });
        expect(result.query_app).toBe('ap2004');
      });

      it('redacts sensitive query parameters', () => {
        const result = extractAuditableRequest(undefined, { token: 'secret-token' });
        expect(result.query_token).toBe('[REDACTED:12chars]');
      });
    });

    describe('combined body and query', () => {
      it('handles both body and query parameters', () => {
        const result = extractAuditableRequest(
          { app: 'ap2004', imsi: '001010000000001' },
          { operation: 'GetStatus', token: 'my-token' },
        );

        expect(result.app).toBe('ap2004');
        expect(result.imsi).toBe('[REDACTED:15chars]');
        expect(result.query_operation).toBe('GetStatus');
        expect(result.query_token).toBe('[REDACTED:8chars]');
      });
    });

    describe('edge cases', () => {
      it('handles empty body', () => {
        const result = extractAuditableRequest({}, undefined);
        expect(result).toEqual({});
      });

      it('handles undefined body', () => {
        const result = extractAuditableRequest(undefined, undefined);
        expect(result).toEqual({});
      });

      it('handles array values in redacted fields', () => {
        const result = extractAuditableRequest({ token: ['a', 'b', 'c'] as unknown as string }, undefined);
        expect(result.token).toBe('[REDACTED:array:3items]');
      });

      it('handles object values in redacted fields', () => {
        const result = extractAuditableRequest({ token: { nested: 'value' } as unknown as string }, undefined);
        expect(result.token).toBe('[REDACTED:object]');
      });

      it('handles numeric values in redacted fields', () => {
        const result = extractAuditableRequest({ imsi: 12345 as unknown as string }, undefined);
        expect(result.imsi).toBe('[REDACTED:number]');
      });

      it('is case-insensitive for field matching', () => {
        const result = extractAuditableRequest({
          APP: 'ap2004',
          IMSI: '001010000000001',
          Token: 'secret',
        }, undefined);

        // Field names are preserved in output, but matching is case-insensitive
        expect(result.APP).toBe('ap2004');
        expect(result.IMSI).toBe('[REDACTED:15chars]');
        expect(result.Token).toBe('[REDACTED:6chars]');
      });
    });

    describe('real-world request examples', () => {
      it('handles typical initial EAP-AKA request', () => {
        const result = extractAuditableRequest({
          app: 'ap2004',
          terminal_id: '12345678901234',
          entitlement_version: '2',
          imsi: '001010000000001',
        }, undefined);

        expect(result).toEqual({
          app: 'ap2004',
          terminal_id: '12345678901234',
          entitlement_version: '2',
          imsi: '[REDACTED:15chars]',
        });
      });

      it('handles typical EAP-AKA RT2 request', () => {
        const eapRelay = 'AgEALBcBAAAQEhQWGBocHh8hIyUnKSsuMA=='; // Example base64
        const result = extractAuditableRequest({
          app: 'ap2004',
          terminal_id: '12345678901234',
          entitlement_version: '2',
          eap_relay: eapRelay,
        }, undefined);

        expect(result.app).toBe('ap2004');
        expect(result.eap_relay).toBe(`[REDACTED:${eapRelay.length}chars]`);
        expect(result.eap_relay).not.toContain('AgEA'); // Actual content not leaked
      });

      it('handles typical token-based request', () => {
        const token = 'a1b2c3d4e5f6'.repeat(5); // 60 char token
        const result = extractAuditableRequest({
          app: 'ap2006',
          terminal_id: '12345678901234',
          entitlement_version: '2',
          token,
          operation: 'ManageSubscription',
          operation_type: 1,
        }, undefined);

        expect(result).toEqual({
          app: 'ap2006',
          terminal_id: '12345678901234',
          entitlement_version: '2',
          token: '[REDACTED:60chars]',
          operation: 'ManageSubscription',
          operation_type: 1,
        });
      });
    });
  });
});
