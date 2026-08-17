package main

import (
	"database/sql"
	"log"
	"net/http"
	"sort"
	"strings"
)

type propertyRow struct {
	Slug        string
	Name        string
	Description string
}

func queryPropertyRows(rulesDB *sql.DB, table string) ([]propertyRow, error) {
	rows, err := rulesDB.Query(`SELECT slug, name, description FROM ` + table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var properties []propertyRow
	for rows.Next() {
		var p propertyRow
		if err := rows.Scan(&p.Slug, &p.Name, &p.Description); err != nil {
			return nil, err
		}
		properties = append(properties, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(properties, func(i, k int) bool { return sortKey(properties[i].Name) < sortKey(properties[k].Name) })
	return properties, nil
}

func (s *server) handleProperties(w http.ResponseWriter, r *http.Request) {
	weaponProperties, err := queryPropertyRows(s.rulesDB, "weapon_properties")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query weapon properties:", err)
		return
	}
	armorProperties, err := queryPropertyRows(s.rulesDB, "armor_properties")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query armor properties:", err)
		return
	}

	s.render(w, "properties.html", map[string]any{
		"Title": "Equipment Properties", "WeaponProperties": weaponProperties, "ArmorProperties": armorProperties,
	})
}

type propertyWeaponRow struct {
	Name   string
	Detail sql.NullString
}

type propertyDetail struct {
	Slug        string
	Name        string
	Description string
	Weapons     []propertyWeaponRow
}

func (s *server) handlePropertyDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	// The two property kinds are self-describing by slug prefix
	// ('property/...' vs 'armor-property/...', see 0012/0016's INSERTs),
	// so one route can serve both without an ambiguous lookup.
	propertyTable, junctionTable, label := "weapon_properties", "equipment_properties", "Weapons"
	if strings.HasPrefix(slug, "armor-property/") {
		propertyTable, junctionTable, label = "armor_properties", "equipment_armor_properties", "Armor"
	}

	var p propertyDetail
	err := s.rulesDB.QueryRow(
		`SELECT slug, name, description FROM `+propertyTable+` WHERE slug = ?`, slug,
	).Scan(&p.Slug, &p.Name, &p.Description)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query property detail:", err)
		return
	}

	rows, err := s.rulesDB.Query(`
		SELECT e.name, ep.detail
		FROM `+junctionTable+` ep JOIN equipment e ON e.slug = ep.equipment_slug
		WHERE ep.property_slug = ?`, slug)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query property equipment:", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var wp propertyWeaponRow
		if err := rows.Scan(&wp.Name, &wp.Detail); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan property equipment:", err)
			return
		}
		p.Weapons = append(p.Weapons, wp)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("property equipment rows:", err)
		return
	}
	sort.Slice(p.Weapons, func(i, k int) bool { return sortKey(p.Weapons[i].Name) < sortKey(p.Weapons[k].Name) })

	s.render(w, "property_detail.html", map[string]any{"Title": p.Name, "Property": p, "EquipmentLabel": label})
}
