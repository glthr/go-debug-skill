--[[
minted -- enable the minted environment for code listings in beamer and latex.
Adapted from https://github.com/pandoc/lua-filters/tree/master/minted
Supports attributes: highlightlines, firstnumber, highlightcolor (for breakpoint highlighting).
]]
local minted_default_block_language = "text"
local minted_block_attributes = {"autogobble"}

local function minted_language(block)
  if #block.classes > 0 then
    return block.classes[1]
  end
  return minted_default_block_language
end

local function minted_attributes(block)
  local opts = {}
  for _, a in ipairs(minted_block_attributes) do
    table.insert(opts, a)
  end
  -- Add attributes from block.attr.attributes (e.g. highlightlines=42 firstnumber=40)
  if block.attr and block.attr.attributes then
    for k, v in pairs(block.attr.attributes) do
      if k == "highlightlines" then
        table.insert(opts, string.format("highlightlines={%s}", v))
      elseif k == "firstnumber" then
        table.insert(opts, string.format("firstnumber=%s", v))
      elseif k == "highlightcolor" then
        table.insert(opts, string.format("highlightcolor=%s", v))
      end
    end
  end
  return table.concat(opts, ",")
end

function CodeBlock(block)
  if FORMAT == "beamer" or FORMAT == "latex" then
    local language = minted_language(block)
    local attributes = minted_attributes(block)
    local raw = string.format(
      "\\begin{minted}[%s]{%s}\n%s\n\\end{minted}",
      attributes,
      language,
      block.text
    )
    return pandoc.RawBlock("latex", raw)
  end
  return block
end

return {{CodeBlock = CodeBlock}}
