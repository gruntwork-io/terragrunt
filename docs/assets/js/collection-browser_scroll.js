$(document).ready(function () {

  const getElementForDataSelector = function (parentElement, selectorName, elementName) {
    const selector = parentElement.data(selectorName);
    if (!selector) {
      throw new Error(`You must specify a 'data-${selectorName}' attribute for '${elementName}'.`);
    }

    const element = $(selector);
    if (element.length !== 1) {
      throw new Error(`Expected one element that matched selector '${selector}' for '${elementName}' but got ${element.length}`);
    }

    return element;
  };

  // Move the TOC on the left side of the page with the user as the user scrolls down, so the TOC is always visible.
  // Only start moving the TOC once the user has scrolled past the element specified in scroll-after-selector. Stop
  // moving it at the bottom of the content.
  const moveToCWithScrolling = function () {
    const sidebar = $(".js-scroll-with-user");

    const scrollAfter = getElementForDataSelector(sidebar, 'scroll-after-selector', 'moveTocWithScrolling');
    const scrollUntil = getElementForDataSelector(sidebar, 'scroll-until-selector', 'moveTocWithScrolling');

    const scrollPosition = $(window).scrollTop();
    const scrollAfterHeightBottom = scrollAfter.offset().top + scrollAfter.innerHeight();

    const contentHeight = scrollUntil.innerHeight() + scrollAfterHeightBottom;
    const sidebarHeight = sidebar.height();
    const sidebarBottomPos = scrollPosition + sidebarHeight;

    // Only start moving the TOC once we're past the scroll-after item
    if (scrollPosition >= scrollAfterHeightBottom) {
      // Stop moving the TOC when we're at the bottom of the content
      if (sidebarBottomPos >= contentHeight) {
        sidebar.removeClass('fixed');
        sidebar.addClass('bottom');
      } else {
        sidebar.addClass('fixed');
        sidebar.removeClass('bottom');
      }
    } else {
      sidebar.removeClass('fixed');
      sidebar.removeClass('bottom');
    }
  };

  // Show a dot next to the part of the TOC where the user has scrolled to. We can't use bootstrap's built-in ScrollSpy
  // because with Bootstrap 3.3.7, it only works with a Bootstrap Nav, whereas our TOC is auto-generated and does not
  // use Bootstrap Nav classes/markup.
  const scrollSpy = function () {
    const content = $(".js-scroll-spy");

    const nav = getElementForDataSelector(content, 'scroll-spy-nav-selector', 'scrollSpy');

    const allNavLinks = nav.find('a');
    allNavLinks.removeClass('selected');

    // Only consider an item in view if it's visible in the top 20% of the screen
    const buffer = $(window).height() / 5;
    const scrollPosition = $(window).scrollTop();
    const contentHeadings = content.find('h2, h3, h4');
    const visibleHeadings = contentHeadings.filter((index, el) => scrollPosition + buffer >= $(el).offset().top);

    if (visibleHeadings.length > 0) {
      const selectedHeading = visibleHeadings.last();
      const selectedHeadingId = selectedHeading.attr('id');

      if (selectedHeadingId) {
        const hash = `#${selectedHeadingId}`;
        const selectedNavLink = nav.find(`a[href$='${hash}']`);
        if (selectedNavLink.length > 0) {
          selectedNavLink.addClass('selected');

          const allTopLevelNavListItems = nav.find('.sectlevel1 > li');

          const parentNavListItem = selectedNavLink.parents('.sectlevel2').parent();
          const topLevelNavListItem = selectedNavLink.parents('.sectlevel1');
        }
      }
    }
  };



  $(window).scroll(moveToCWithScrolling);
  $(moveToCWithScrolling);

  $(window).scroll(scrollSpy);
  $(scrollSpy);

  $('.post-detail img').on('click', function () {
    window.open(this.src, '_blank')
  })
});
