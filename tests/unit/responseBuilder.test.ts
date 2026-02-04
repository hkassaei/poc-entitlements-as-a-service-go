import { describe, it, expect } from 'vitest';
import { buildJsonResponse } from '../../src/protocol/jsonBuilder.js';
import { buildXmlResponse } from '../../src/protocol/xmlBuilder.js';
import { buildVoWiFiConfig } from '../../src/services/vowifi.js';
import { buildVoLTEConfig } from '../../src/services/volte.js';
import { buildSmsOipConfig } from '../../src/services/smsoip.js';
import type { ServiceEntitlementResponse } from '../../src/protocol/responseTypes.js';

const SAMPLE_RESPONSE: ServiceEntitlementResponse = {
  version: '1',
  validity: 172800,
  token: 'test-token-abc123',
  applications: [
    {
      appId: 'ap2004',
      entitlementStatus: 1,
      addrStatus: 1,
      tcStatus: 0,
      provStatus: 3,
      addresses: [
        { addrType: '1', addr: 'epdg.operator.com' },
        { addrType: '1', addr: 'pcscf.operator.com' },
      ],
    },
  ],
};

describe('JSON Builder', () => {
  it('produces correct TS.43 envelope structure', () => {
    const result = buildJsonResponse(SAMPLE_RESPONSE) as Record<string, unknown>;

    expect(result.Vers).toEqual({ version: '1', validity: '172800' });
    expect(result.Token).toEqual({ token: 'test-token-abc123' });
    expect(result.ap2004).toBeDefined();
  });

  it('serializes application block with string values', () => {
    const result = buildJsonResponse(SAMPLE_RESPONSE) as Record<string, unknown>;
    const app = result.ap2004 as Record<string, unknown>;

    expect(app.EntitlementStatus).toBe('1');
    expect(app.AddrStatus).toBe('1');
    expect(app.TC_Status).toBe('0');
    expect(app.ProvStatus).toBe('3');
  });

  it('serializes address array with indexed keys', () => {
    const result = buildJsonResponse(SAMPLE_RESPONSE) as Record<string, unknown>;
    const app = result.ap2004 as Record<string, unknown>;
    const addr = app.Addr as Record<string, { AddrType: string; Addr: string }>;

    expect(addr['1']).toEqual({ AddrType: '1', Addr: 'epdg.operator.com' });
    expect(addr['2']).toEqual({ AddrType: '1', Addr: 'pcscf.operator.com' });
  });

  it('omits addresses when not present', () => {
    const response: ServiceEntitlementResponse = {
      ...SAMPLE_RESPONSE,
      applications: [
        { appId: 'ap2004', entitlementStatus: 0, addrStatus: 0 },
      ],
    };

    const result = buildJsonResponse(response) as Record<string, unknown>;
    const app = result.ap2004 as Record<string, unknown>;

    expect(app.Addr).toBeUndefined();
  });

  it('includes extra params (VoLTE)', () => {
    const response: ServiceEntitlementResponse = {
      ...SAMPLE_RESPONSE,
      applications: [
        {
          appId: 'ap2003',
          entitlementStatus: 1,
          extraParams: { VoLTE_Entitled: '1', VoNR_Entitled: '1' },
        },
      ],
    };

    const result = buildJsonResponse(response) as Record<string, unknown>;
    const app = result.ap2003 as Record<string, unknown>;

    expect(app.VoLTE_Entitled).toBe('1');
    expect(app.VoNR_Entitled).toBe('1');
  });

  it('includes ServiceFlow_URL when present', () => {
    const response: ServiceEntitlementResponse = {
      ...SAMPLE_RESPONSE,
      applications: [
        {
          appId: 'ap2004',
          entitlementStatus: 0,
          serviceFlowUrl: 'https://operator.com/terms',
        },
      ],
    };

    const result = buildJsonResponse(response) as Record<string, unknown>;
    const app = result.ap2004 as Record<string, unknown>;

    expect(app.ServiceFlow_URL).toBe('https://operator.com/terms');
  });
});

describe('XML Builder', () => {
  it('produces valid WAP-Provisioning XML', () => {
    const xml = buildXmlResponse(SAMPLE_RESPONSE);

    expect(xml).toContain('<?xml version="1.0"?>');
    expect(xml).toContain('<wap-provisioningdoc version="1.1">');
    expect(xml).toContain('</wap-provisioningdoc>');
  });

  it('includes VERS characteristic', () => {
    const xml = buildXmlResponse(SAMPLE_RESPONSE);

    expect(xml).toContain('<characteristic type="VERS">');
    expect(xml).toContain('name="version" value="1"');
    expect(xml).toContain('name="validity" value="172800"');
  });

  it('includes TOKEN characteristic', () => {
    const xml = buildXmlResponse(SAMPLE_RESPONSE);

    expect(xml).toContain('<characteristic type="TOKEN">');
    expect(xml).toContain('name="token" value="test-token-abc123"');
  });

  it('includes APPLICATION characteristic with parms', () => {
    const xml = buildXmlResponse(SAMPLE_RESPONSE);

    expect(xml).toContain('<characteristic type="APPLICATION">');
    expect(xml).toContain('name="AppID" value="ap2004"');
    expect(xml).toContain('name="EntitlementStatus" value="1"');
    expect(xml).toContain('name="TC_Status" value="0"');
    expect(xml).toContain('name="ProvStatus" value="3"');
  });

  it('includes ADDR sub-characteristics', () => {
    const xml = buildXmlResponse(SAMPLE_RESPONSE);

    expect(xml).toContain('<characteristic type="ADDR">');
    expect(xml).toContain('name="AddrType" value="1"');
    expect(xml).toContain('name="Addr" value="epdg.operator.com"');
    expect(xml).toContain('name="Addr" value="pcscf.operator.com"');
  });

  it('escapes special XML characters', () => {
    const response: ServiceEntitlementResponse = {
      ...SAMPLE_RESPONSE,
      token: 'token&with<special>"chars',
      applications: [],
    };

    const xml = buildXmlResponse(response);

    expect(xml).toContain('token&amp;with&lt;special&gt;&quot;chars');
    expect(xml).not.toContain('token&with<special>"chars');
  });
});

describe('Service Handlers', () => {
  describe('VoWiFi (ap2004)', () => {
    it('returns addresses when ENABLED', () => {
      const config = buildVoWiFiConfig(1, 3, 1);
      expect(config.appId).toBe('ap2004');
      expect(config.entitlementStatus).toBe(1);
      expect(config.addrStatus).toBe(1);
      expect(config.addresses).toBeDefined();
      expect(config.addresses!.length).toBeGreaterThan(0);
    });

    it('omits addresses when DISABLED', () => {
      const config = buildVoWiFiConfig(0, 0, 0);
      expect(config.addrStatus).toBe(0);
      expect(config.addresses).toBeUndefined();
    });

    it('uses custom addresses from configData', () => {
      const configData = {
        addresses: [{ addrType: '2', addr: '10.0.0.1' }],
      };
      const config = buildVoWiFiConfig(1, 3, 1, configData);
      expect(config.addresses).toEqual([{ addrType: '2', addr: '10.0.0.1' }]);
    });

    it('includes serviceFlowUrl when TC requires acceptance', () => {
      const configData = { serviceFlowUrl: 'https://operator.com/terms' };
      const config = buildVoWiFiConfig(0, 1, 2, configData);
      expect(config.serviceFlowUrl).toBe('https://operator.com/terms');
    });
  });

  describe('VoLTE (ap2003)', () => {
    it('includes VoLTE/VoNR entitled params when ENABLED', () => {
      const config = buildVoLTEConfig(1, 3, 1);
      expect(config.appId).toBe('ap2003');
      expect(config.extraParams!.VoLTE_Entitled).toBe('1');
      expect(config.extraParams!.VoNR_Entitled).toBe('1');
      expect(config.addresses).toBeDefined();
    });

    it('sets VoLTE/VoNR to 0 when DISABLED', () => {
      const config = buildVoLTEConfig(0, 0, 0);
      expect(config.extraParams!.VoLTE_Entitled).toBe('0');
      expect(config.extraParams!.VoNR_Entitled).toBe('0');
    });
  });

  describe('SMSoIP (ap2005)', () => {
    it('returns addresses when ENABLED', () => {
      const config = buildSmsOipConfig(1, 0, 0);
      expect(config.appId).toBe('ap2005');
      expect(config.addrStatus).toBe(1);
      expect(config.addresses).toBeDefined();
    });

    it('omits addresses when DISABLED', () => {
      const config = buildSmsOipConfig(0, 0, 0);
      expect(config.addrStatus).toBe(0);
      expect(config.addresses).toBeUndefined();
    });
  });
});
