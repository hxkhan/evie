local function split(s, sep)
	local out = {}
	local n = 0
	for part in string.gmatch(s, "[^" .. sep .. "]+") do
		n = n + 1
		out[n] = part
	end
	return out
end

local function main()
	local base = "alpha,beta,gamma,delta,epsilon"
	local total = 0
	for i = 0, 39999 do
		local s = base .. "," .. i
		local parts = split(s, ",")
		local joined = table.concat(parts, "-")
		local replaced = string.gsub(joined, "beta", "BETA")
		if string.find(replaced, "gamma", 1, true) ~= nil then
			total = total + #replaced
		end
	end
	return total
end

print(main())
