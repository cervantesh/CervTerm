local record cervterm
  record Term
    write: function(Term, string)
    notify: function(Term, string)
    size: function(Term): integer, integer
    cursor: function(Term): integer, integer
    title: function(Term): string
    line: function(Term, integer): string
  end
  record Events
    output: function(Term, string)
    title: function(Term, string)
    bell: function(Term)
  end
end
return cervterm
