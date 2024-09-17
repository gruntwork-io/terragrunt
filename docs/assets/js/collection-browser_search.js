/**
 * Javascript for the Collection Browser search.
 *
 * TOC:
 *  - FILTER FUNCTIONS - functions to extract the terms from DOM element(s) and use them in search engine to show/hide elements.
 *  - MAIN - INITIALIZE - initializes Browser Search and registers actions (click etc.) on filter components.
 *  - SEARCH ENGINE - here is the logic to show & hide elements according to filters terms.
 *  - OTHER
 */
(function () {

  /** ********************************************************************* **/
  /** *********                 FILTER FUNCTIONS                  ********* **/
  /** ********************************************************************* **/

  /**
   * Note
   * This function is wrapped in a "debounce" so that if the user is typing quickly, we aren't trying to run searches
   * (and fire Google Analytics events!) on every key stroke, but only when they pause from typing.
   * @type {Function}
   */
  const searchInputFilter = debounce(function (event) {
    const target = $(event.currentTarget)
    const collectionName = target.data('collection_name')
    filterSearchData(collectionName)
  }, 250);


  /** ********************************************************************* **/
  /** *********               MAIN - INITIALIZE                   ********* **/
  /** ********************************************************************* **/

  /**
  * Bind actions to search input and tags.
  * If you want to add more filter components (like cloud providers on gruntwork.io/guides)
  * you can register actions on them here.
  */
  function initializeCbSearch(el) {
    const collectionName = el.data('collection_name')

    /* SEARCH INPUT box on page */
    $('#cb-search-box-'+collectionName).on("keyup", searchInputFilter);

    /* Triggered when TAGS filter checkboxes are checked */
    $(document)
      .on('click', '[data-collection_name="'+collectionName+'"] .tags', function () {
        filterSearchData(collectionName);
      });
  }

  /* Find collection browser's search component on the page and initialize: */
  if($('.cb-search-cmp').length > 0) {
    $('.cb-search-cmp').each(function () {
      initializeCbSearch($(this))
      showNoResults(false)
    })
  }

  /** ********************************************************************* **/
  /** *********                 SEARCH ENGINE                     ********* **/
  /** ********************************************************************* **/

  /**
   * Filters posts/docs/entries against search input and tags.
   * It always gets terms from both: search input and tags whenever any of them changed.
   */
  function filterSearchData(collectionName) {
    // Get data from all filter components
    // a) Get Search input:
    const searchInputValue = $('#cb-search-box-'+collectionName).val().toLowerCase().split(" ").filter(v => v != '')
    // b) Get tags:
    let checkedTags = []
    $('[data-collection_name="'+collectionName+'"] input[type="checkbox"]:checked')
      .each(function () {
        checkedTags.push($(this).val())
      })

    // If there is no filter terms, show all posts. Otherwise, filter posts:
    if (searchInputValue.length === 0 && checkedTags.length === 0) {
      showNoResults(false)
      showAll()
    } else {
      // Get the list of posts and categories  to show
      const toShow = filterDocs(collectionName, searchInputValue, checkedTags)

      // If there is no posts to show, display no-results component
      if (toShow.docs.length === 0) {
        hideAll()
        showNoResults(true)
      } else {
        // Hide no-results component
        showNoResults(false)
        // Hide all elements
        hideAll()
        // Show elements
        toShow.docs.forEach(docId => {
          showDoc(docId)
        })
        toShow.categories.forEach(catId => {
          showCategory(catId)
        })
      }
    }
  }

  /**
   * Filter docs (entries) against search input and checked tags
   * It returns list of documents and categories to show (satisfying search terms).
   */
  function filterDocs(collectionName, searchInputValue, checkedTags) {
    // Fetch docs data
    const docs = fetchDocsData(collectionName)
    let toShowDocs = []
    let toShowCategories = []
    // Check each doc's data against search value and selected tags:
    docs.forEach(doc => {
      if (containsText(doc, searchInputValue) && containsTag(doc, checkedTags)) {
        toShowDocs.push(doc.id)
        if (toShowCategories.indexOf(doc.category) === -1) {
          toShowCategories.push(doc.category.replace(/\s+/g, '-'))
        }
      }
    })
    return { docs: toShowDocs, categories: toShowCategories }
  }

  function containsTerms(content, terms) {
    let allMatches = true
    terms.forEach(term => {
      if (content.indexOf(term.toLowerCase()) < 0) {
        allMatches = false
      }
    })
    return allMatches
  }

  function containsText(doc, terms) {
    const content = doc.text || doc.title + " " + doc.excerpt + " " + doc.category + " " + doc.content + " " + doc.tags
    return containsTerms(content, terms)
  }

  function containsTag(doc, terms) {
    const content = doc.tags
    return containsTerms(content, terms)
  }

  /**
   * Function to fetch posts/docs data.
   * Now it gets from window, but it can be transformed to get it from API.
   */
  function fetchDocsData(collectionName) {
    return window['bc_'+collectionName+'Entries']
  }

  /** ********************************************************************* **/
  /** *********                 OTHER                             ********* **/
  /** ********************************************************************* **/

  // Returns a function, that, as long as it continues to be invoked, will not be
  // triggered. The function will be called after it stops being called for N
  // milliseconds. If `immediate` is passed, trigger the function on the leading
  // edge, instead of the trailing. Ensures a given task doesn't fire so often
  // that it bricks browser performance. From:
  // https://davidwalsh.name/javascript-debounce-function
  function debounce(func, wait, immediate) {
    let timeout
    return function () {
      const context = this,
        args = arguments
      const later = function () {
        timeout = null
        if (!immediate)
          func.apply(context, args)
      };
      const callNow = immediate && !timeout
      clearTimeout(timeout)
      timeout = setTimeout(later, wait)
      if (callNow)
        func.apply(context, args)
    };
  }

  /**
   * Functions to show & hide items on the page
   */
  function showAll() {
    $('.cb-doc-card').show()
    $('.category-head').show()
    $('.categories ul li').show()
  }

  function hideAll() {
    $('.cb-doc-card').hide()
    $('.category-head').hide()
    $('.categories ul li').hide()
  }

  function showDoc(docId) {
    $('#' + docId + '.cb-doc-card').show()
  }

  function showCategory(categoryId) {
    $(`.categories ul [data-category=${categoryId}]`).show()
    $(`#${categoryId}.category-head`).show()
  }

  /**
   * Show / hide no-results component
   */
   function showNoResults(state) {
     if (state) {
       $('#no-matches').show()
     } else {
       $('#no-matches').hide()
     }
   }

}());
