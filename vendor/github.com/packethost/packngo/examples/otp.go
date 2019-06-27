package examples

import (
	"bufio"
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/packethost/packngo"
)

func GetPacketOtpSms(c *packngo.Client) (string, error) {
	_, err := c.TwoFactorAuth.ReceiveSms()
	if err != nil {
		return "", err
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter SMS code: ")
	otp, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return otp[:len(otp)-1], nil
}

func SeedPacketOtpApp(c *packngo.Client) (string, error) {
	otpUri, _, err := c.TwoFactorAuth.SeedApp()
	if err != nil {
		return "", err
	}
	log.Println("OTP URI:", otpUri)
	u, err := url.Parse(otpUri)
	if err != nil {
		return "", err
	}
	q := u.Query()
	log.Println("Secret for 2FA App:", q["secret"][0])

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter Packet 2FA App code: ")
	otp, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return otp[:len(otp)-1], nil
}

func GetOtpApp(c *packngo.Client) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter 2FA App code: ")
	otp, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return otp[:len(otp)-1], nil
}

func TestSMSEnable() {
	c, err := packngo.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	otp, err := GetPacketOtpSms(c)
	if err != nil {
		log.Fatal(err)
	}

	_, err = c.TwoFactorAuth.EnableSms(otp)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("SMS enabled")

	otp, err = GetPacketOtpSms(c)
	if err != nil {
		log.Fatal(err)
	}

	_, err = c.TwoFactorAuth.DisableSms(otp)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("SMS disabled")
}

func TestAppEnable() {
	c, err := packngo.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	otp, err := SeedPacketOtpApp(c)
	if err != nil {
		log.Fatal(err)
	}

	_, err = c.TwoFactorAuth.EnableApp(otp)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("App enabled")

	otp, err = GetOtpApp(c)
	if err != nil {
		log.Fatal(err)
	}

	_, err = c.TwoFactorAuth.DisableApp(otp)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("App disabled")
}
