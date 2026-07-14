local record cervterm
  record Term
    write: function(Term, string)
    notify: function(Term, string)
    selection: function(Term): string
    copy: function(Term, string)
    clipboard: function(Term): string
    scroll: function(Term, integer): boolean
    scroll_to_bottom: function(Term)
    scrollback: function(Term): integer
    size: function(Term): integer, integer
    cursor: function(Term): integer, integer
    title: function(Term): string
    set_title: function(Term, string)
    line: function(Term, integer): string
    line_wrapped: function(Term, integer): boolean
  end
  record Events
    output: function(Term, string)
    title: function(Term, string)
    bell: function(Term)
  end
end
return cervterm
