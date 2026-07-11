local cervterm = require("cervterm")

local config = {
  events = {
    output = function(term: cervterm.Term, data: string)
      if data:find("error") then
        term:notify("saw an error")
      end
    end,
    title = function(term: cervterm.Term, title: string)
      term:notify("title: " .. title)
    end,
    bell = function(term: cervterm.Term)
      term:notify("ding")
    end,
  },
}
return config
