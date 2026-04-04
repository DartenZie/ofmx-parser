-- Data processing based on openmaptiles.org schema
-- https://openmaptiles.org/schema/
-- Copyright (c) 2016, KlokanTech.com & OpenMapTiles contributors.
-- Used under CC-BY 4.0

--------
-- Alter these lines to control which languages are written for place/streetnames
--
-- Preferred language can be (for example) "en" for English, "de" for German, or nil to use OSM's name tag:
preferred_language = nil
-- This is written into the following vector tile attribute (usually "name:latin"):
preferred_language_attribute = "name:latin"
-- If OSM's name tag differs, then write it into this attribute (usually "name_int"):
default_language_attribute = "name_int"
-- Also write these languages if they differ - for example, { "de", "fr" }
additional_languages = { }
--------

-- Enter/exit Tilemaker
function init_function(name,is_first)
end
function exit_function()
end

-- Implement Sets in tables
function Set(list)
  local set = {}
  for _, l in ipairs(list) do set[l] = true end
  return set
end

-- Meters per pixel if tile is 256x256
ZRES5  = 4891.97
ZRES6  = 2445.98
ZRES7  = 1222.99
ZRES8  = 611.5
ZRES9  = 305.7
ZRES10 = 152.9
ZRES11 = 76.4
ZRES12 = 38.2
ZRES13 = 19.1

-- Used to express that a feature should not end up the vector tiles
INVALID_ZOOM = 99

node_keys = { "place" }

-- Get admin level which the place node is capital of.
-- Returns nil in case of invalid capital and for places which are not capitals.
function capitalLevel(capital)
  local capital_al = tonumber(capital) or 0
  if capital == "yes" then
    capital_al = 2
  end
  if capital_al == 0 then
    return nil
  end
  return capital_al
end

-- Calculate rank for place nodes
-- place: value of place=*
-- popuplation: population as number
-- capital_al: result of capitalLevel()
function calcRank(place, population, capital_al)
  local rank = 0
  if capital_al and capital_al >= 2 and capital_al <= 4 then
    rank = capital_al
    if population > 3 * 10^6 then
      rank = rank - 2
    elseif population > 1 * 10^6 then
      rank = rank - 1
    elseif population < 100000 then
      rank = rank + 2
    elseif population < 50000 then
      rank = rank + 3
    end
    -- Safety measure to avoid place=village/farm/... appear early (as important capital) because a mapper added capital=yes/2/3/4
    if place ~= "city" then
      rank = rank + 3
      -- Decrease rank further if it is not even a town.
      if place ~= "town" then
        rank = rank + 2
      end
    end
    return rank
  end
  if place ~= "city" and place ~= "town" then
    return nil
  end
  if population > 3 * 10^6 then
    return 1
  elseif population > 1 * 10^6 then
    return 2
  elseif population > 500000 then
    return 3
  elseif population > 200000 then
    return 4
  elseif population > 100000 then
    return 5
  elseif population > 75000 then
    return 6
  elseif population > 50000 then
    return 7
  elseif population > 25000 then
    return 8
  elseif population > 10000 then
    return 9
  end
  return 10
end

function node_function()
  -- Write 'place'
  -- note that OpenMapTiles has a rank for countries (1-3), states (1-6) and cities (1-10+);
  --   we could potentially approximate it for cities based on the population tag
  local place = Find("place")
  if place ~= "" then
    local mz = 13
    local pop = tonumber(Find("population")) or 0
    local capital = capitalLevel(Find("capital"))
    local rank = calcRank(place, pop, capital)

    if     place == "continent"     then mz=0
    elseif place == "country"       then
      if     pop>50000000 then rank=1; mz=1
      elseif pop>20000000 then rank=2; mz=2
      else                     rank=3; mz=3 end
    elseif place == "state"         then mz=4
    elseif place == "province"         then mz=5
    elseif place == "city"          then mz=5
    elseif place == "town" and pop>8000 then mz=7
    elseif place == "town"          then mz=8
    elseif place == "village" and pop>2000 then mz=9
    elseif place == "village"       then mz=10
    elseif place == "borough"       then mz=10
    elseif place == "suburb"        then mz=11
    elseif place == "quarter"       then mz=12
    elseif place == "hamlet"        then mz=12
    elseif place == "neighbourhood" then mz=13
    elseif place == "isolated_dwelling" then mz=13
    elseif place == "locality"      then mz=13
    elseif place == "island"      then mz=12
    end

    Layer("place", false)
    Attribute("class", place)
    MinZoom(mz)
    if rank then AttributeInteger("rank", rank) end
    if capital then AttributeInteger("capital", capital) end
    if place=="country" then
      local iso_a2 = Find("ISO3166-1:alpha2")
      while iso_a2 == "" do
        local rel, role = NextRelation()
        if not rel then break end
        if role == 'label' then
          iso_a2 = FindInRelation("ISO3166-1:alpha2")
        end
      end
      Attribute("iso_a2", iso_a2)
    end
    SetNameAttributes()
    return
  end
end

-- Process way tags

majorRoadValues = Set { "motorway", "trunk", "primary" }
z9RoadValues  = Set { "secondary" }
z11RoadValues   = Set { "tertiary" }
manMadeRoadValues = Set { "pier", "bridge" }
pathValues      = Set { }
railwayClasses  = { rail="rail", narrow_gauge="rail", preserved="rail", funicular="rail", subway="transit", light_rail="transit", monorail="transit", tram="transit" }

landuseKeys     = Set { "railway", "residential" }
landcoverKeys   = { wood="wood", forest="wood",
                    fell="grass", grassland="grass", grass="grass", heath="grass", meadow="grass", allotments="grass", park="grass", village_green="grass", recreation_ground="grass", scrub="grass", shrubbery="grass", tundra="grass", garden="grass", golf_course="grass", park="grass" }

waterClasses    = Set { "river", "riverbank", "stream", "canal", "drain", "ditch", "dock" }
waterwayClasses = Set { "stream", "river", "canal", "drain", "ditch" }

way_keys  = { "landuse", "natural", "waterway", "highway", "tunnel", "bridge", "intermittent" }

function write_to_transportation_layer(minzoom, highway_class, is_rail, is_road)
  Layer("transportation", false)
  Attribute("class", highway_class)
  MinZoom(minzoom)

  -- Service
  if (is_rail) then Attribute("service", service) end
end

function way_function()
  local highway      = Find("highway")
  local waterway     = Find("waterway")
  local natural      = Find("natural")
  local landuse      = Find("landuse")
  local leisure      = Find("leisure")
  local amenity      = Find("amenity")
  local tourism      = Find("tourism")
  local tunnel       = Find("tunnel")
  local bridge       = Find("bridge")
  local intermittent = Find("intermittent")
  local is_closed    = IsClosed()

  -- Roads ('transportation' and 'transportation_name')
  if highway ~= "" then
    local h = highway
    local is_road = true

    local minzoom = INVALID_ZOOM
    if majorRoadValues[h]        then minzoom = 4
    elseif h == "trunk"          then minzoom = 5
    elseif highway == "primary"  then minzoom = 7
    elseif z9RoadValues[h]       then minzoom = 9
    elseif z11RoadValues[h]      then minzoom = 11
    end

    -- Write to layer
    if minzoom <= 14 then
      write_to_transportation_layer(minzoom, h, false, is_road)
    end
  end

  -- Set 'waterway'
  if waterwayClasses[waterway] and not is_closed then
    if waterway == "river" then
      Layer("waterway", false)
    end
    if Find("intermittent") == "yes" then AttributeInteger("intermittent", 1) else AttributeInteger("intermittent", 0) end
  end

  -- Set 'water'
  if natural == "water" or landuse == "reservoir"  or landuse=="basin" or waterClasses[waterway] then
    if Find("covered")=="yes" or not is_closed then return end
    local class="lake"; if waterway~="" then class="river" end
    if class=="lake" and Find("wikidata")=="Q192770" then return end
    Layer("water", true)
    SetMinZoomByArea(way)
    Attribute("class",class)

    if Find("intermittent")=="yes" then Attribute("intermittent",1) end

    return -- in case we get any landuse processing
  end
  
  -- Set 'landcover' (from landuse, natural, leisure)
  local l = landuse
  if l == "" then l = natural end
  if l == "" then l = leisure end
  if landcoverKeys[l] then
    Layer("landcover", true)
    SetMinZoomByArea()
    Attribute("class", landcoverKeys[l])
    if l=="wetland" then Attribute("subclass", Find("wetland"))
    else Attribute("subclass", l) end

  -- Set 'landuse'
  else
    if l == "" then l = amenity end
    if l == "" then l = tourism end
    if landuseKeys[l] then
      Layer("landuse", true)
      Attribute("class", l)
      if l=="residential" then
        if Area()<ZRES8^2 then MinZoom(8)
        else SetMinZoomByArea() end
      else MinZoom(11) end
    end
  end
end

-- ==========================================================
-- Common functions

-- Set name attributes on any object
function SetNameAttributes()
  local name = Find("name"), iname
  local main_written = name
  -- if we have a preferred language, then write that (if available), and additionally write the base name tag
  if preferred_language and Holds("name:"..preferred_language) then
    iname = Find("name:"..preferred_language)
    Attribute(preferred_language_attribute, iname)
    if iname~=name and default_language_attribute then
      Attribute(default_language_attribute, name)
    else main_written = iname end
  else
    Attribute(preferred_language_attribute, name)
  end
  -- then set any additional languages
  for i,lang in ipairs(additional_languages) do
    iname = Find("name:"..lang)
    if iname=="" then iname=name end
    if iname~=main_written then Attribute("name:"..lang, iname) end
  end
end

-- Set minimum zoom level by area
function SetMinZoomByArea()
  SetMinZoomByAreaWithLimit(0)
end

-- Set minimum zoom level by area but not below given minzoom
function SetMinZoomByAreaWithLimit(minzoom)
  local area=Area()
  if     minzoom <= 6 and area>ZRES5^2  then MinZoom(6)
  elseif minzoom <= 7 and area>ZRES6^2  then MinZoom(7)
  elseif minzoom <= 8 and area>ZRES7^2  then MinZoom(8)
  elseif minzoom <= 9 and area>ZRES8^2  then MinZoom(9)
  elseif minzoom <= 10 and area>ZRES9^2  then MinZoom(10)
  elseif minzoom <= 11 and area>ZRES10^2 then MinZoom(11)
  elseif minzoom <= 12 and area>ZRES11^2 then MinZoom(12)
  elseif minzoom <= 13 and area>ZRES12^2 then MinZoom(13)
  else                                        MinZoom(14)
  end
end
