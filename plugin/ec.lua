if vim.g.loaded_ec == 1 then
  return
end
vim.g.loaded_ec = 1

local ec = require("ec")
ec.setup()

vim.api.nvim_create_user_command("Ec", function(opts)
  ec.open(opts.fargs)
end, { nargs = "*", complete = "file" })
