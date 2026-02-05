export const AppId = {
  ap2003: 'Voice-over-Cellular (VoLTE/VoNR)',
  ap2004: 'VoWiFi (Wi-Fi Calling)',
  ap2005: 'SMSoIP (SMS over IP)',
  ap2006: 'ODSA Companion (eSIM companion device)',
  ap2009: 'ODSA Primary (eSIM primary device)',
  ap2010: 'Data Plan Information',
  ap2011: 'Server-Initiated ODSA',
  ap2012: 'Direct Carrier Billing',
  ap2013: 'Private User Identity',
  ap2014: 'Device and User Info',
  ap2015: 'App Authentication',
  ap2016: 'Satellite Mode',
} as const;

export type AppIdValue = keyof typeof AppId;
