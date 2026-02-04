/**
 * TS.43 XML Response Builder (WAP-Provisioning)
 *
 * Converts the internal ServiceEntitlementResponse into
 * WAP-Provisioning XML format per GSMA TS.43.
 *
 * No external XML library — the format is rigid and predictable.
 */

import type { ServiceEntitlementResponse, ApplicationConfig } from './responseTypes.js';

function escapeXml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function parm(name: string, value: string): string {
  return `    <parm name="${escapeXml(name)}" value="${escapeXml(value)}"/>`;
}

function parmIndent(name: string, value: string, indent: string): string {
  return `${indent}<parm name="${escapeXml(name)}" value="${escapeXml(value)}"/>`;
}

/**
 * Build a WAP-Provisioning XML response string.
 */
export function buildXmlResponse(response: ServiceEntitlementResponse): string {
  const lines: string[] = [
    '<?xml version="1.0"?>',
    '<wap-provisioningdoc version="1.1">',
  ];

  // VERS characteristic
  lines.push('  <characteristic type="VERS">');
  lines.push(parm('version', response.version));
  lines.push(parm('validity', String(response.validity)));
  lines.push('  </characteristic>');

  // TOKEN characteristic
  lines.push('  <characteristic type="TOKEN">');
  lines.push(parm('token', response.token));
  lines.push('  </characteristic>');

  // APPLICATION characteristics
  for (const app of response.applications) {
    lines.push(...buildApplicationCharacteristic(app));
  }

  lines.push('</wap-provisioningdoc>');
  return lines.join('\n');
}

function buildApplicationCharacteristic(app: ApplicationConfig): string[] {
  const lines: string[] = [];
  const indent = '    ';

  lines.push('  <characteristic type="APPLICATION">');
  lines.push(parm('AppID', app.appId));
  lines.push(parm('EntitlementStatus', String(app.entitlementStatus)));

  if (app.addrStatus !== undefined) {
    lines.push(parm('AddrStatus', String(app.addrStatus)));
  }

  if (app.tcStatus !== undefined) {
    lines.push(parm('TC_Status', String(app.tcStatus)));
  }

  if (app.provStatus !== undefined) {
    lines.push(parm('ProvStatus', String(app.provStatus)));
  }

  if (app.serviceFlowUrl) {
    lines.push(parm('ServiceFlow_URL', app.serviceFlowUrl));
  }

  if (app.extraParams) {
    for (const [key, value] of Object.entries(app.extraParams)) {
      lines.push(parm(key, value));
    }
  }

  if (app.addresses && app.addresses.length > 0) {
    for (const addr of app.addresses) {
      lines.push(`${indent}<characteristic type="ADDR">`);
      lines.push(parmIndent('AddrType', addr.addrType, `${indent}  `));
      lines.push(parmIndent('Addr', addr.addr, `${indent}  `));
      lines.push(`${indent}</characteristic>`);
    }
  }

  lines.push('  </characteristic>');
  return lines;
}
