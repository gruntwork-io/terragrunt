import globalCss from '../../styles/global.css?raw';

export async function GET() {
  return new Response(globalCss, {
    headers: {
      'Content-Type': 'text/css',
      'Cache-Control': 'public, max-age=3600',
    },
  });
}
