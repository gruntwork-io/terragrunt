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
})
