import pagefindCss from '../../styles/pagefind.css?raw';

export async function GET() {
  return new Response(pagefindCss, {
    headers: {
      'Content-Type': 'text/css',
      'Cache-Control': 'public, max-age=3600',
    },
  });
}
