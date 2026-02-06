import type { Static } from 'typebox';
import type { EntitlementRequestBody, EntitlementRequestQuery } from './requestSchemas.js';

export type EntitlementPostRequest = Static<typeof EntitlementRequestBody>;
export type EntitlementGetRequest = Static<typeof EntitlementRequestQuery>;
