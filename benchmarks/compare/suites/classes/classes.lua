local Adder = {}
Adder.__index = Adder

function Adder.new(base, step)
	return setmetatable({ base = base, step = step }, Adder)
end

function Adder:adjust(x)
	self.base = (self.base + self.step) % 1000
	return self.base + x
end

function Adder:compute(x)
	return self:adjust(x) + self.step
end

local Scaler = {}
Scaler.__index = Scaler

function Scaler.new(base, factor)
	return setmetatable({ base = base, factor = factor }, Scaler)
end

function Scaler:adjust(x)
	self.base = (self.base + x) % 1000
	return self.base * self.factor
end

function Scaler:compute(x)
	return self:adjust(x) - self.factor
end

local function main()
	local items = {}
	for i = 0, 199 do
		if i % 2 == 0 then
			items[i + 1] = Adder.new(i, (i % 7) + 1)
		else
			items[i + 1] = Scaler.new(i, (i % 5) + 1)
		end
	end

	local total = 0
	for r = 0, 1999 do
		local x = r % 13
		for j = 1, 200 do
			total = total + items[j]:compute(x)
		end
	end
	return total
end

print(main())
