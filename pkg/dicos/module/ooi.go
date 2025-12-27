package module

import (
	"github.com/jpfielding/dicos.go/pkg/dicos/tag"
)

// OOIOwnerModule represents the OOI Owner Module (NEMA IIC 1 v04-2023 Section 3)
// Identifies the owner of the Object of Inspection
type OOIOwnerModule struct {
	// Owner Identification
	OwnerID     string // Owner unique identifier
	OwnerName   string // Owner name (person or organization)
	OwnerIDType string // Type of ID: PASSPORT, BADGE, TICKET, etc.

	// Owner Category
	OwnerCategory string // PASSENGER, CREW, EMPLOYEE, VISITOR
}

// OOIModule represents the Object of Inspection Module (NEMA IIC 1 v04-2023 Section 4)
// Describes the object being scanned (bag, cargo, person, etc.)
type OOIModule struct {
	// OOI Identification
	OOIID    string // Unique identifier for the OOI
	OOIType  string // BAG, CARGO, PERSON, VEHICLE
	OOISize  string // CABIN, CHECKED, OVERSIZE
	OOILabel string // Label or tag identifier

	// OOI Classification
	OOICategory string // CARRY_ON, CHECKED_BAGGAGE, AIR_CARGO
	ContentType string // PASSENGER_BELONGINGS, COMMERCIAL_GOODS

	// Scan Context
	ScanType        string // PRIMARY, SECONDARY, TERTIARY
	InspectionMode  string // AUTOMATIC, OPERATOR_ASSISTED
	ScreeningDevice string // Device identifier
}

// ItineraryModule represents travel/routing information (NEMA IIC 1 v04-2023 Section 4.2)
type ItineraryModule struct {
	// Flight/Journey Information
	FlightNumber      string
	DepartureAirport  string // IATA code
	ArrivalAirport    string // IATA code
	DepartureDateTime string // DICOM DT format
	ArrivalDateTime   string // DICOM DT format

	// Carrier Information
	CarrierName string
	CarrierCode string // IATA airline code

	// Connection Information
	ConnectionAirports []string // Intermediate stops
}

// NewOOIOwnerModule creates an OOIOwnerModule with defaults
func NewOOIOwnerModule() *OOIOwnerModule {
	return &OOIOwnerModule{}
}

// NewOOIModule creates an OOIModule with defaults
func NewOOIModule() *OOIModule {
	return &OOIModule{
		OOIType:        "BAG",
		OOICategory:    "CHECKED_BAGGAGE",
		InspectionMode: "AUTOMATIC",
	}
}

// NewItineraryModule creates an ItineraryModule with defaults
func NewItineraryModule() *ItineraryModule {
	return &ItineraryModule{}
}

// ToTags converts OOIOwnerModule to DICOM tag elements
func (m *OOIOwnerModule) ToTags() []IODElement {
	var elements []IODElement

	if m.OwnerID != "" {
		elements = append(elements, IODElement{Tag: tag.OOIOwnerID, Value: m.OwnerID})
	}
	if m.OwnerName != "" {
		elements = append(elements, IODElement{Tag: tag.OOIOwnerName, Value: m.OwnerName})
	}
	if m.OwnerIDType != "" {
		elements = append(elements, IODElement{Tag: tag.OOIOwnerIDType, Value: m.OwnerIDType})
	}
	if m.OwnerCategory != "" {
		elements = append(elements, IODElement{Tag: tag.OOIOwnerCategory, Value: m.OwnerCategory})
	}

	return elements
}

// ToTags converts OOIModule to DICOM tag elements
func (m *OOIModule) ToTags() []IODElement {
	var elements []IODElement

	if m.OOIID != "" {
		elements = append(elements, IODElement{Tag: tag.OOIID, Value: m.OOIID})
	}
	if m.OOIType != "" {
		elements = append(elements, IODElement{Tag: tag.OOITypeAttr, Value: m.OOIType})
	}
	if m.OOISize != "" {
		elements = append(elements, IODElement{Tag: tag.OOISizeAttr, Value: m.OOISize})
	}
	if m.OOILabel != "" {
		elements = append(elements, IODElement{Tag: tag.OOILabel, Value: m.OOILabel})
	}

	return elements
}

// ToTags converts ItineraryModule to DICOM tag elements
func (m *ItineraryModule) ToTags() []IODElement {
	var elements []IODElement

	if m.FlightNumber != "" {
		elements = append(elements, IODElement{Tag: tag.FlightNumber, Value: m.FlightNumber})
	}
	if m.DepartureAirport != "" {
		elements = append(elements, IODElement{Tag: tag.DepartureAirport, Value: m.DepartureAirport})
	}
	if m.ArrivalAirport != "" {
		elements = append(elements, IODElement{Tag: tag.ArrivalAirport, Value: m.ArrivalAirport})
	}
	if m.CarrierName != "" {
		elements = append(elements, IODElement{Tag: tag.CarrierName, Value: m.CarrierName})
	}
	if m.CarrierCode != "" {
		elements = append(elements, IODElement{Tag: tag.CarrierCode, Value: m.CarrierCode})
	}

	return elements
}
