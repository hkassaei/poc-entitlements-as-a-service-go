import { Type } from 'typebox';

const AppIdSchema = Type.Union([
  Type.Literal('ap2003'),
  Type.Literal('ap2004'),
  Type.Literal('ap2005'),
  Type.Literal('ap2006'),
  Type.Literal('ap2009'),
  Type.Literal('ap2010'),
  Type.Literal('ap2011'),
  Type.Literal('ap2012'),
  Type.Literal('ap2013'),
  Type.Literal('ap2014'),
  Type.Literal('ap2015'),
  Type.Literal('ap2016'),
]);

const OdsaOperationSchema = Type.Union([
  Type.Literal('CheckEligibility'),
  Type.Literal('ManageSubscription'),
  Type.Literal('ManageService'),
  Type.Literal('AcquireConfiguration'),
  Type.Literal('AcquireTemporaryToken'),
  Type.Literal('GetOperatorToken'),
  Type.Literal('AcquirePlan'),
]);

export const EntitlementRequestBody = Type.Object({
  app: AppIdSchema,
  terminal_id: Type.String({ minLength: 14, maxLength: 16 }),
  entitlement_version: Type.String({ pattern: '^[0-9]+$' }),
  imsi: Type.Optional(Type.String({ pattern: '^[0-9]{15}$' })),
  imei: Type.Optional(Type.String({ pattern: '^[0-9]{14,16}$' })),
  terminal_vendor: Type.Optional(Type.String()),
  terminal_model: Type.Optional(Type.String()),
  terminal_type: Type.Optional(Type.String()),
  token: Type.Optional(Type.String()),
  eap_relay: Type.Optional(Type.String()),
  operation: Type.Optional(OdsaOperationSchema),
  operation_type: Type.Optional(Type.Integer()),
  accept_content_type: Type.Optional(
    Type.Union([Type.Literal('xml'), Type.Literal('json')]),
  ),
});

export const EntitlementRequestQuery = Type.Object({
  app: AppIdSchema,
  terminal_id: Type.String({ minLength: 14, maxLength: 16 }),
  entitlement_version: Type.String({ pattern: '^[0-9]+$' }),
  imsi: Type.Optional(Type.String({ pattern: '^[0-9]{15}$' })),
  imei: Type.Optional(Type.String({ pattern: '^[0-9]{14,16}$' })),
  terminal_vendor: Type.Optional(Type.String()),
  terminal_model: Type.Optional(Type.String()),
  terminal_type: Type.Optional(Type.String()),
  token: Type.Optional(Type.String()),
  eap_relay: Type.Optional(Type.String()),
  operation: Type.Optional(OdsaOperationSchema),
  operation_type: Type.Optional(Type.String({ pattern: '^[0-9]+$' })),
  accept_content_type: Type.Optional(
    Type.Union([Type.Literal('xml'), Type.Literal('json')]),
  ),
});
