import type { APIRoute } from 'astro';
import { llmsTxtResponse } from '@lib/llms-txt';

export const prerender = true;

export const GET: APIRoute = (context) => llmsTxtResponse(context, 'small');
