const navExceptions = [
  ['use-cases/keep-your-terraform-code-dry/', 'getting-started'],
  ['use-cases/keep-your-remote-state-configuration-dry/', 'getting-started'],
  ['use-cases/keep-your-cli-flags-dry/', 'getting-started'],
  ['use-cases/execute-terraform-commands-on-multiple-modules-at-once/', 'getting-started'],
  ['use-cases/work-with-multiple-aws-accounts/', 'getting-started']
]

$(document).ready(function () {
  $('#toc-toggle-open').on('click', function () {
    $('.col-md-2-5').addClass('opened')
    $('body').addClass('modal-opened')
  })

  $('#toc-toggle-close').on('click', function () {
    $('.col-md-2-5').removeClass('opened')
    $('body').removeClass('modal-opened')
  })

  $('#toc a').on('click', function (){
    $('.col-md-2-5').removeClass('opened')
    $('body').removeClass('modal-opened')
  })

  /* Collapsing toc */
  $('#toc ul ul').addClass('collapse')

  // Change initial icon for nav without children:
  $('#toc .nav-collapse-handler').each(function () {
    if ($(this).siblings('ul').length === 0) {
      $(this).find('.glyphicon').removeClass('glyphicon-triangle-bottom')
      $(this).find('.glyphicon').addClass('glyphicon-chevron-down')
      $(this).addClass('no-children')
    }
  })

  // Expand / collpase on click
  $('#toc .nav-collapse-handler').on('click', function() {
    toggleNav($(this))
  })

  $(docSidebarInitialExpand)

  // Links to use cases are duplicated in navigation bar. To expand only those
  // from "Getting started" and not from "Features", we handle them as navExceptions
  // and expand them manually.
  // Expand navigation for use cases:
  for (var i=0; i<navExceptions.length; i++) {
    expandToByURL(navExceptions[i][0], navExceptions[i][1])
  }

  if (window.location.pathname === '/use-cases/' || window.location.pathname === '/terragrunt/use-cases/') {
    expandToById('cat-nav-id-use-cases-')
  }

})

// Expand / collpase on click
function toggleNav(el) {
  if (el.hasClass('collapsed')) {
    el.removeClass('collapsed')
    el.siblings('ul').collapse('show')
  } else {
    el.addClass('collapsed')
    el.siblings('ul').collapse('hide')
  }
}

const docSidebarInitialExpand = function () {
  const toc = $('#toc')
  const pathname = window.location.pathname
  const hash = window.location.hash
  var isException = $.grep(navExceptions, function(e){ return (pathname+hash).includes(e[0]) })
  if (isException.length === 0) {
    toc.find('a[href="'+pathname+hash+'"]').each(function(i, nav) {
      $(nav).parents('ul').each(function(i, el) {
        $(el).collapse('show')
        $(el).siblings('span.nav-collapse-handler:not(.no-children)').removeClass('collapsed')
      })
      $(nav).siblings('span.nav-collapse-handler:not(.no-children)').removeClass('collapsed')
      $(nav).siblings('ul').collapse('show')
    })
  }
}

function expandToByURL(url, parentFilter) {
  if ((window.location.pathname + window.location.hash).includes(url)) {
    const toc = $('#toc')
    var selector = 'a[href*="'+url+'"]'
    toc.find(selector).each(function(i, nav) {
      $(nav).siblings('ul').collapse("show")
      $(nav).siblings('span.nav-collapse-handler:not(.no-children)').removeClass('collapsed')
      $(nav).parents('ul').each(function(i, el) {
        $(el).collapse("show")
        $(el).siblings('span.nav-collapse-handler:not(.no-children)').removeClass('collapsed')
      })
    })
  }
}

function expandToById(id) {
  const toc = $('#toc')
  toc.find('ul[id^="'+id+'"]').each(function(i, nav) {
    $(nav).parents('ul[id^="cat-nav-id"]').collapse("show")
    $(nav).collapse("show")
  })
}
