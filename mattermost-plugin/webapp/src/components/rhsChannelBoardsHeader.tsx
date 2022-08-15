// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.
import React  from 'react'
import {FormattedMessage, IntlProvider} from 'react-intl'

import {getMessages} from '../../../../webapp/src/i18n'
import {getLanguage} from '../../../../webapp/src/store/language'
import {getCurrentChannel} from '../../../../webapp/src/store/channels'
import {useAppSelector} from '../../../../webapp/src/store/hooks'

const RHSChannelBoardsHeader = () => {
    const appBarIconURL = (window as any).baseURL + '/public/app-bar-icon.png'
    const currentChannel = useAppSelector(getCurrentChannel)
    const language = useAppSelector<string>(getLanguage)

    if (!currentChannel) {
        return null
    }

    return (
        <IntlProvider
            locale={language.split(/[_]/)[0]}
            messages={getMessages(language)}
        >
            <div>
                <img
                    className='boards-rhs-header-logo'
                    src={appBarIconURL}
                />
                <span>
                    <FormattedMessage
                        id='rhs-channel-boards-header.title'
                        defaultMessage='Boards'
                    />
                </span>
                <span className='style--none sidebar--right__title__subtitle'>{currentChannel.display_name}</span>
            </div>
        </IntlProvider>
    )
}

export default RHSChannelBoardsHeader
