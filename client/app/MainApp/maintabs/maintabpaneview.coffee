class MainTabPane extends KDTabPaneView

  constructor:(options, data)->

    @id        or= options.id
    options.type = options.behavior

    super options, data
