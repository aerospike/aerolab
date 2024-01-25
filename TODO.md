# TODO for webui

## Split required
1. release single-user version
2. next release: multi-user support

## Required
* Bug with new naming conventions in docker - if old name exists, will attempt to start new name and fail as image not found
* Agi - after it stopped, on restart, will it remember which instance size was used for a given volume? If not, will it be aware of need to size, and will it ask the monitor to size (like a file on the volume with sizing information)?
* add support for special "owner/user" header to auto-set owner and account the job to a specific user; if header present, do not allow owner edit in webui
* change weblog to contain timestamp as well
  * change path: ./weblog/user-owner/items.log
* history - configurable max-per-user completed history items with background job cleaner (never account in cleaner for active job count); auto max based on day/week count instead of actual job count?
* clear option should just keep is timestamp of when clear was pressed - don't show items prior to timestamp - cookie so browser-based choice
* non-interactive - never attempt to ask user if they are sure, disable `less`
* home page, with config options (and getting started page saying "try these 1,2,3..." and/or quick-access common items like cluster create)
* special inventory page(s) for all list items
* fill the actions top-level dropdown, show success/fail/in-progress in pending-action section
* struct tag to mark some elements in webui should not be forms but show as "not implemented in webui" - like webui, rest commands
* implement console for all attach commands
  * also add new installable feature - webconsole - featuring ttyd - enabled by default if installed from webui
  * ttyd local on aerolab, and make that run ssh
* struct tag: required - this should show a star next to required items, and JS should, on submit attempt, check if all required items are filled in
* implement "simple" mode which will have the list of options greatly reduced, present "simple/full" slider
  * definitions should be a list of items, like `aerolab config defaults` without the values
  * should have a sane default selection
  * user should be able to specify their own list as either replacement, addition or removal
    * eg:
      * +Cluster.Create.NoSetHostname
      * -Cluster.Create.Nodes
      * allow in specification to state:
        * -all - remove all default options and start empty
        * +all - add all options to start (+all without any other will be like a full interface)
  * add option to disable full mode  view option toggle altogether

## TBC
* new form type: upload file - tmp file upload and fill into form
  * some elements need this, like file upload, download, etc

## Wishlist
* allow loading a historical command into the form from previous run log or from command-line command (reverse-form-load)
* right-sidebar - quick settings and inventory listing

## WIP Notes
* pending-action-count: innerText
* classes: badge-warning - in progress; badge-danger - some actions failed; badge-success - all actions succeeded
* pending-action-icon: classes: "far fa-bell" - nothing in progress; make spinny wheel if actions in progress exist
* pendingActionShowAll(): under pending-user-action-list:
  * cleanup
	* remove all items with class dropdown-divider
	* remove all items with class dropdown-item (and their sub-items)
  * add this layout, use spinner icon for running actions, red X for failed, and green V for success
```
  <div class="dropdown-divider"></div>
  <a href="#" class="dropdown-item">
    <i class="fas fa-envelope mr-2"></i> 4 new messages
    <span class="float-right text-muted text-sm">3 mins</span>
    <br><span class="text-muted text-sm">Robert Glonek</span><span class="text-muted float-right text-sm">rglonek@aerospike.com</span>
  </a>
```
  * each one of these should open a modal which will show the output of aerolab command that ran - reuse same modal, just use a different action ID
  * display all running and last X finished runs (configurable on server side)
  * footer should have "clear" button
  * clear should only clear notifications for current user - i.e. server needs to keep the track on truncate of notifications (by finish time) per user
