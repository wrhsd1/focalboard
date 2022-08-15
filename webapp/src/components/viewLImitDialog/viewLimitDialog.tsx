// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import React, {useEffect} from 'react'

import './viewLimitDialog.scss'
import {FormattedMessage, useIntl} from 'react-intl'

import Dialog from '../dialog'

import upgradeImage from '../../../static/upgrade.png'
import {useAppSelector} from '../../store/hooks'
import {getMe} from '../../store/users'
import {Utils} from '../../utils'
import Button from '../../widgets/buttons/button'
import octoClient from '../../octoClient'
import telemetryClient, {TelemetryActions, TelemetryCategory} from '../../telemetry/telemetryClient'
import {getCurrentBoard} from '../../store/boards'

export type PublicProps = {
    onClose: () => void
}

export type Props = PublicProps & {
    showNotifyAdminSuccess: () => void
}

export const ViewLimitModal = (props: Props): JSX.Element => {
    const me = useAppSelector(getMe)
    const isAdmin = me ? Utils.isAdmin(me.roles) : false
    const intl = useIntl()

    const board = useAppSelector(getCurrentBoard)

    useEffect(() => {
        telemetryClient.trackEvent(TelemetryCategory, TelemetryActions.ViewLimitReached, {board: board.id})
    }, [])

    const heading = (
        <FormattedMessage
            id='ViewLimitDialog.Heading'
            defaultMessage='Views per board limit reached'
        />
    )

    const regularUserSubtext = (
        <FormattedMessage
            id='ViewLimitDialog.Subtext.RegularUser'
            defaultMessage='Notify your Admin to upgrade to our Professional or Enterprise plan to have unlimited views per boards, unlimited cards, and more.'
        />
    )

    const regularUserPrimaryButtonText = intl.formatMessage({id: 'ViewLimitDialog.PrimaryButton.Title.RegularUser', defaultMessage: 'Notify Admin'})

    const adminSubtext = (
        <React.Fragment>
            <FormattedMessage
                id='ViewLimitDialog.Subtext.Admin'
                defaultMessage='Upgrade to our Professional or Enterprise plan to have unlimited views per boards, unlimited cards, and more.'
            />
            <a
                href='https://mattermost.com/pricing/'
                target='_blank'
                rel='noreferrer'
            >
                <FormattedMessage
                    id='ViewLimitDialog.Subtext.Admin.PricingPageLink'
                    defaultMessage='Learn more about our plans.'
                />
            </a>
        </React.Fragment>
    )

    const adminPrimaryButtonText = intl.formatMessage({id: 'ViewLimitDialog.PrimaryButton.Title.Admin', defaultMessage: 'Upgrade'})

    const subtext = isAdmin ? adminSubtext : regularUserSubtext
    const primaryButtonText = isAdmin ? adminPrimaryButtonText : regularUserPrimaryButtonText

    const handlePrimaryButtonAction = async () => {
        telemetryClient.trackEvent(TelemetryCategory, TelemetryActions.ViewLimitCTAPerformed, {board: board.id})

        if (isAdmin) {
            (window as any)?.openPricingModal()()
        } else {
            await octoClient.notifyAdminUpgrade()
            props.showNotifyAdminSuccess()
        }

        props.onClose()
    }

    return (
        <Dialog
            className='ViewLimitDialog'
            onClose={props.onClose}
        >
            <div className='ViewLimitDialog_body'>
                <img
                    src={Utils.buildURL(upgradeImage, true)}
                    alt={intl.formatMessage({id: 'ViewLimitDialog.UpgradeImg.AltText', defaultMessage: 'upgrade image'})}
                />
                <h2 className='header text-heading5'>
                    {heading}
                </h2>
                <p className='text-heading1'>
                    {subtext}
                </p>
            </div>
            <div className='ViewLimitDialog_footer'>
                <Button
                    size={'medium'}
                    className='cancel'
                    onClick={props.onClose}
                >
                    <FormattedMessage
                        id='ConfirmationDialog.cancel-action'
                        defaultMessage='Cancel'
                    />
                </Button>
                <Button
                    size='medium'
                    className='primaryAction'
                    emphasis='primary'
                    onClick={handlePrimaryButtonAction}
                >
                    {primaryButtonText}
                </Button>
            </div>
        </Dialog>
    )
}
