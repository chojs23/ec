local M = {}

local defaults = {
	cmd = "ec",
	open_cmd = "tabnew",
	cwd = nil,
	float = true,
	close_on_exit = true,
}

local default_float = {
	width = 0.92,
	height = 0.86,
	border = "rounded",
	title = "ec",
	title_pos = "center",
	zindex = 50,
}

M.config = vim.deepcopy(defaults)

local function resolve_cmd(args)
	local cmd = {}
	if type(M.config.cmd) == "string" then
		cmd = { M.config.cmd }
	elseif type(M.config.cmd) == "table" then
		cmd = vim.deepcopy(M.config.cmd)
	else
		vim.notify("ec: invalid cmd config", vim.log.levels.ERROR)
		return nil
	end

	if args and #args > 0 then
		vim.list_extend(cmd, args)
	end
	return cmd
end

local function resolve_cwd()
	if type(M.config.cwd) == "function" then
		return M.config.cwd()
	end
	if type(M.config.cwd) == "string" and M.config.cwd ~= "" then
		return M.config.cwd
	end
	return vim.fn.getcwd()
end

local function resolve_float_config()
	local float = M.config.float
	if not float then
		return nil
	end
	if float == true then
		float = {}
	end
	if type(float) ~= "table" then
		vim.notify("ec: invalid float config", vim.log.levels.ERROR)
		return nil
	end
	return vim.tbl_deep_extend("force", vim.deepcopy(default_float), float)
end

local function clamp_size(value, total, min_size)
	local size = value
	if size <= 1 then
		size = math.floor(total * size)
	else
		size = math.floor(size)
	end
	local max_size = math.max(total - 2, 1)
	size = math.min(size, max_size)
	size = math.max(size, math.min(min_size, max_size))
	return size
end

local function open_float_window()
	local float = resolve_float_config()
	if not float then
		return nil
	end

	local columns = vim.o.columns
	local available_lines = vim.o.lines - vim.o.cmdheight - 1
	if available_lines < 1 then
		available_lines = vim.o.lines
	end

	local width = clamp_size(float.width, columns, 20)
	local height = clamp_size(float.height, available_lines, 6)
	local col = math.floor((columns - width) / 2)
	local row = math.floor((available_lines - height) / 2)

	local buf = vim.api.nvim_create_buf(false, true)
	local win_config = {
		relative = "editor",
		width = width,
		height = height,
		col = col,
		row = row,
		style = "minimal",
		border = float.border,
		zindex = float.zindex,
	}
	if float.title and float.title ~= "" then
		win_config.title = float.title
		win_config.title_pos = float.title_pos
	end

	local winid = vim.api.nvim_open_win(buf, true, win_config)
	vim.api.nvim_set_option_value("number", false, { win = winid })
	vim.api.nvim_set_option_value("relativenumber", false, { win = winid })
	vim.api.nvim_set_option_value("signcolumn", "no", { win = winid })
	if float.winblend then
		vim.api.nvim_set_option_value("winblend", float.winblend, { win = winid })
	end
	return buf, winid
end

local function close_terminal(bufnr, winid)
	if winid and vim.api.nvim_win_is_valid(winid) then
		vim.api.nvim_win_close(winid, true)
	end
	if bufnr and vim.api.nvim_buf_is_valid(bufnr) then
		vim.api.nvim_buf_delete(bufnr, { force = true })
	end
end

function M.setup(opts)
	M.config = vim.tbl_deep_extend("force", vim.deepcopy(defaults), opts or {})
end

function M.open(args)
	local cmd = resolve_cmd(args or {})
	if not cmd then
		return
	end

	if vim.fn.executable(cmd[1]) ~= 1 then
		vim.notify("ec: executable not found: " .. cmd[1], vim.log.levels.ERROR)
		return
	end

	local bufnr, winid = open_float_window()
	if not bufnr then
		vim.cmd(M.config.open_cmd)
		bufnr = vim.api.nvim_get_current_buf()
		winid = vim.api.nvim_get_current_win()
	end
	local cwd = resolve_cwd()
	local ok, job_id = pcall(vim.fn.jobstart, cmd, {
		cwd = cwd,
		term = true,
		on_exit = function(_, code)
			if M.config.close_on_exit and code == 0 then
				vim.schedule(function()
					close_terminal(bufnr, winid)
				end)
			end
		end,
	})
	if not ok or job_id <= 0 then
		vim.notify("ec: failed to start terminal job", vim.log.levels.ERROR)
		return
	end
	vim.cmd("startinsert")
end

-- TODO: add non-interactive auto mode command.

return M
