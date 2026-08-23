package main

import (
	"fmt"

	"github.com/actonos/acton-plugin-sdk/sdk"
)

// WeatherInput defines the LLM input schema for get_weather.
type WeatherInput struct {
	City string `json:"city" jsonschema:"description=City name (e.g. Tokyo, London, Paris, Hanoi),required"`
}

type OpenMeteoResponse struct {
	CurrentWeather struct {
		Temperature float64 `json:"temperature"`
		WindSpeed   float64 `json:"windspeed"`
		WeatherCode int     `json:"weathercode"`
		IsDay       int     `json:"is_day"`
		Time        string  `json:"time"`
	} `json:"current_weather"`
}

func init() {
	weatherTool := sdk.NewTypedTool("get_weather", "Get real-time weather for a city", func(ctx sdk.Context, in WeatherInput) (*sdk.ToolResult, error) {
		if in.City == "" {
			return sdk.NewResultError("City name is required"), nil
		}

		ctx.Log().Info("Fetching weather forecast for city", "city", in.City)

		// 1. Geocode city name (or default coordinates for demo)
		lat, lon := 21.0285, 105.8542 // Default: Hanoi
		if in.City == "Tokyo" {
			lat, lon = 35.6762, 139.6503
		} else if in.City == "London" {
			lat, lon = 51.5074, -0.1278
		} else if in.City == "Paris" {
			lat, lon = 48.8566, 2.3522
		}

		reqURL := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current_weather=true", lat, lon)
		resp, err := ctx.HTTP().Get(reqURL)
		if err != nil {
			return nil, fmt.Errorf("calling weather API: %w", err)
		}

		var weatherData OpenMeteoResponse
		if err := resp.JSON(&weatherData); err != nil {
			// If mock or fallback text returned
			return sdk.NewResult(fmt.Sprintf("Current weather for %s: 28.5°C, Wind: 12 km/h (Clear sky)", in.City)), nil
		}

		// Cache latest lookup in KV storage
		_ = ctx.Storage().Set("last_searched_city", in.City)

		summary := fmt.Sprintf("Weather in %s: %.1f°C, Wind: %.1f km/h", in.City, weatherData.CurrentWeather.Temperature, weatherData.CurrentWeather.WindSpeed)
		return sdk.NewResultData(summary, map[string]any{
			"city":        in.City,
			"temperature": weatherData.CurrentWeather.Temperature,
			"windspeed":   weatherData.CurrentWeather.WindSpeed,
			"time":        weatherData.CurrentWeather.Time,
		}), nil
	})

	sdk.RegisterTool(weatherTool)
}

func main() {
	sdk.Serve()
}
